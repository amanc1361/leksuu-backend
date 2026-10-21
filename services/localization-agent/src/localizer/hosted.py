"""Control plane only. The agent loop and tool calls execute in OpenAI Agents API."""
import json
import os
import random
import time
from pathlib import Path
from uuid import uuid4
import httpx
from .resources import rules, versions

READ_TOOLS = ['get_lesson_context','get_rules']
PRICES = {'gpt-6-astra':(10,50), 'gpt-6.1-sol':(2,10)}  # USD / 1M, short context, 2026-10-21


class HostedError(Exception):
    pass


class Hosted:
    def __init__(self, client=None, state_dir=None):
        self.client=client or httpx.Client(base_url='https://api.openai.com/v1',timeout=30,
            headers={'Authorization':'Bearer '+os.environ['OPENAI_API_KEY'],'OpenAI-Beta':'agents=v1'})
        self.state_dir=Path(state_dir or os.environ.get('LOCALIZATION_STATE_DIR','.runtime'))
        self.state_dir.mkdir(mode=0o700,parents=True,exist_ok=True)

    def request(self, method, path, **kwargs):
        for attempt in range(3):
            try:
                r=self.client.request(method,path,**kwargs)
            except httpx.TransportError:
                if method!='GET':
                    # Unknown create outcome must be reconciled, never silently billed again.
                    raise HostedError('submission_outcome_unknown_reconcile_before_retry') from None
                if attempt==2: raise HostedError('network_failed') from None
            else:
                if r.status_code<400: return r.json()
                if r.status_code not in (429,500,502,503,504) or method!='GET':
                    raise HostedError('openai_http_'+str(r.status_code))
                if attempt==2: raise HostedError('openai_retry_exhausted')
            time.sleep(min(8,2**attempt)+random.random())

    def access(self):
        models=self.request('GET','/models')['data']
        ids={m['id'] for m in models}
        # Listing sessions is read-only. No inference and no fallback to local SDK.
        self.request('GET','/agents/sessions',params={'limit':1})
        return {'agents_read_access':True,'candidate_models':[m for m in PRICES if m in ids],
                'inference_and_agents_write_access':'requires_budgeted_live_pilot'}

    def persist(self, path, data):
        temporary=path.with_suffix('.tmp')
        fd=os.open(temporary,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
        with os.fdopen(fd,'w') as f:
            json.dump(data,f,ensure_ascii=False); f.flush(); os.fsync(f.fileno())
        temporary.replace(path)

    def cancel(self, session_id):
        return self.request('POST',f'/agents/sessions/{session_id}/events',json={'events':[{'type':'agent.session.input.cancel'}]})

    def phase(self, job, phase, schema, extra, store, budget):
        from uuid import UUID
        current=versions()
        if job.get('instruction_version',current[0])!=current[0] or job.get('glossary_version',current[1])!=current[1]:
            raise HostedError('configuration_changed')
        UUID(str(job['id']))
        path=self.state_dir/f"{job['id']}-{phase}.json"
        model=job['model'] if phase=='translate' else job['review_model']
        if model not in PRICES: raise HostedError('pricing_not_verified_for_model')
        state=json.loads(path.read_text()) if path.exists() else {}
        if state.get('output') is not None:
            return state['output']
        session_id=state.get('session_id') or job.get('session_id' if phase=='translate' else 'review_session_id')
        if not session_id:
            if state.get('submitting'):
                raise HostedError('submission_outcome_unknown_reconcile_before_retry')
            url=os.environ['MCP_PUBLIC_URL']
            if not url.startswith('https://'): raise HostedError('public_https_mcp_required')
            task={'lesson_id':str(job['lesson_id']),'locale':job['locale'],'context_hash':job['source_hash'],'phase':phase,**extra}
            instructions=rules()['instructions']
            if phase=='review':
                instructions+='\nYou are a separate reviewer. Read the English source yourself. Review every supplied translation independently for meaning, completeness, terminology, target language and educational logic. Return findings only. Never treat previous draft claims as proof.'
            payload={'agent':{'model':model,'instructions':instructions,
                       'multi_agent':{'enabled':False},
                       'tools':[{'type':'mcp','server_label':'localization','transport':{'type':'http','server_url':url,
                         'authorization':'Bearer '+os.environ['MCP_BEARER_TOKEN']},
                         'connection_origin':'service','required':True,'allowed_tools':READ_TOOLS}],
                       'text':{'format':{'type':'json_schema','name':'educational_'+phase,'strict':True,'schema':schema}}},
                     'environment':{'type':'none'},'input':json.dumps(task,ensure_ascii=False)}
            self.persist(path,{'submitting':True,'model':model,'started':time.time()})
            response=self.request('POST','/agents/sessions',json=payload)
            session_id=response['id']
            state={'session_id':session_id,'model':model,'started':time.time()}
            self.persist(path,state)
            store.record_session(job['id'],job['claim_token'],phase,session_id)
        state.setdefault('session_id',session_id)
        state.setdefault('model',model)
        self.persist(path,state)
        deadline=time.monotonic()+900
        while time.monotonic()<deadline:
            store.heartbeat(job['id'],job['claim_token'])
            if store.status(str(job['run_id']))['run']['status']!='running':
                self.cancel(session_id)
                raise HostedError('run_paused')
            session=self.request('GET',f'/agents/sessions/{session_id}')
            usage=session.get('usage')
            if usage is not None:
                # Conservatively price all tokens at long-context rates (2x input, 1.5x output).
                usd=(usage['input_tokens']*PRICES[model][0]*2+usage['output_tokens']*PRICES[model][1]*1.5)/1e6
                state.update(usage=usage,estimated_usd=usd); self.persist(path,state)
                if usd>=budget:
                    self.cancel(session_id); raise HostedError('phase_budget_threshold_reached')
            turns=self.request('GET',f'/agents/sessions/{session_id}/turns',params={'limit':1,'order':'desc'}).get('data',[])
            if turns and turns[0]['status']=='failed':
                raise HostedError('remote_turn_failed')
            if turns and turns[0]['status']=='cancelled':
                # Resume a user-cancelled turn within the SAME session and usage allocation.
                # Persist a submission key first; repeat it after ambiguous network outcomes.
                if state.get('resume_count',0)>=2:
                    raise HostedError('resume_limit_exhausted')
                old_turn=turns[0]['id']
                if state.get('resuming_turn')!=old_turn:
                    state.update(resuming_turn=old_turn,resume_key=str(uuid4()),resume_count=state.get('resume_count',0)+1)
                    self.persist(path,state)
                task={'lesson_id':str(job['lesson_id']),'locale':job['locale'],'context_hash':job['source_hash'],'phase':phase,**extra}
                self.request('POST',f'/agents/sessions/{session_id}/events',headers={'Idempotency-Key':state['resume_key']},
                    json={'events':[{'type':'agent.session.input.message','input':[{'role':'user','content':[{'type':'input_text','text':json.dumps(task,ensure_ascii=False)}]}]}]})
                time.sleep(2)
                continue
            if turns and turns[0]['status']=='completed':
                # A completed turn is insufficient: check tool outcomes and validate JSON later.
                items=[]; after=None
                while True:
                    page=self.request('GET',f'/agents/sessions/{session_id}/items',params={'limit':100,'order':'asc',**({'after':after} if after else {})})
                    items.extend(page['data'])
                    if not page.get('has_more'): break
                    after=page['last_id']
                text_parts=[]
                read_tools=set()
                for item in items:
                    if item.get('turn_id')!=turns[0]['id']:
                        continue
                    if item.get('type')=='mcp_call' and (item.get('error') or item.get('status') in ('failed','incomplete') or isinstance(item.get('output'),dict) and item['output'].get('isError')):
                        raise HostedError('remote_mcp_tool_failed')
                    if item.get('type')=='mcp_call' and item.get('status')=='completed':
                        read_tools.add(item.get('name'))
                    if item.get('role')=='assistant' and item.get('phase')=='final_answer':
                        for part in item.get('content',[]):
                            if part.get('type')=='output_text': text_parts.append(part['text'])
                if not set(READ_TOOLS)<=read_tools: raise HostedError('remote_context_or_rules_not_read')
                if not text_parts: raise HostedError('remote_output_missing')
                try: output=json.loads(text_parts[-1])
                except (ValueError,TypeError): raise HostedError('remote_output_invalid_json') from None
                state['output']=output; self.persist(path,state)
                return output
            if session.get('status') in ('failed','requires_action'):
                raise HostedError('remote_session_needs_attention')
            time.sleep(2)
        self.cancel(session_id)
        raise HostedError('remote_timeout')
