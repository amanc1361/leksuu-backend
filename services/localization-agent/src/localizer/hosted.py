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


CACHED_PRICES = {'gpt-6-astra':1, 'gpt-6.1-sol':0.1}
TRANSIENT_TURN_ERRORS = {'internal_error','server_error','server_overloaded','rate_limit_exceeded','connection_failed','request_timeout'}


def sum_usage(usages):
    if not usages:
        return None
    return {'input_tokens':sum(u['input_tokens'] for u in usages),
            'output_tokens':sum(u['output_tokens'] for u in usages),
            'input_tokens_details':{'cached_tokens':sum((u.get('input_tokens_details') or {}).get('cached_tokens',0) for u in usages)},
            'output_tokens_details':{'reasoning_tokens':sum((u.get('output_tokens_details') or {}).get('reasoning_tokens',0) for u in usages)}}


def usage_cost(usage, model, long_context=False):
    cached=min(usage['input_tokens'],max(0,(usage.get('input_tokens_details') or {}).get('cached_tokens',0)))
    input_rate,output_rate=PRICES[model]
    multiplier=2 if long_context else 1
    return ((usage['input_tokens']-cached)*input_rate*multiplier+
            cached*CACHED_PRICES[model]*multiplier+
            usage['output_tokens']*output_rate*(1.5 if long_context else 1))/1e6


def successful_reads(items, turn_id, lesson_id):
    latest={}; recovered=0
    for item in items:
        if item.get('turn_id')!=turn_id or item.get('type')!='mcp_call':
            continue
        name=item.get('name')
        if name not in READ_TOOLS:
            raise HostedError('unexpected_remote_tool')
        output=item.get('output')
        ok=item.get('status')=='completed' and not item.get('error') and not (isinstance(output,dict) and output.get('isError'))
        arguments=item.get('arguments',{})
        if isinstance(arguments,str):
            try: arguments=json.loads(arguments)
            except ValueError: arguments={}
        if name=='get_lesson_context':
            ok=ok and isinstance(arguments,dict) and arguments.get('lesson_id')==str(lesson_id)
        if not ok:
            recovered+=1
        latest[name]=ok
    if any(not value for value in latest.values()):
        raise HostedError('remote_mcp_tool_failed')
    if not set(READ_TOOLS)<=latest.keys():
        raise HostedError('remote_context_or_rules_not_read')
    return recovered


class HostedError(Exception):
    def __init__(self, code, status_code=None):
        super().__init__(code)
        self.status_code=status_code


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
                    raise HostedError('openai_http_'+str(r.status_code),status_code=r.status_code)
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
            # Recover accounting for output saved by an older controller version.
            if state.get('usage') is None and state.get('session_id'):
                turns=self.request('GET',f"/agents/sessions/{state['session_id']}/turns",params={'limit':100,'order':'desc'}).get('data',[])
                current=sum_usage([t['usage'] for t in turns if t.get('usage') is not None])
                previous=state.get('prior_sessions',[])
                usage=sum_usage([p['usage'] for p in previous if p.get('usage') is not None]+([current] if current is not None else []))
                if usage is not None:
                    conservative=sum(usage_cost(p['usage'],p.get('model',model),True) for p in previous if p.get('usage') is not None)+(usage_cost(current,model,True) if current is not None else 0)
                    standard=sum(usage_cost(p['usage'],p.get('model',model)) for p in previous if p.get('usage') is not None)+(usage_cost(current,model) if current is not None else 0)
                    state.update(usage=usage,estimated_usd=conservative,standard_rate_estimate_usd=standard,
                                 turn_usage_incomplete=any(p.get('usage') is None or p.get('unknown_usage') for p in previous) or any(t.get('usage') is None for t in turns))
                    self.persist(path,state)
                    if state['estimated_usd']>=budget:
                        raise HostedError('phase_budget_threshold_reached')
            return state['output']
        session_id=state.get('session_id') or (None if state.get('restart_after_terminal_failure') else job.get('session_id' if phase=='translate' else 'review_session_id'))
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
                       'text':{'format':{'type':'json_schema','schema':schema}}},
                     'environment':{'type':'none'},'input':json.dumps(task,ensure_ascii=False)}
            self.persist(path,{**state,'submitting':True,'model':model,'started':time.time()})
            try:
                response=self.request('POST','/agents/sessions',json=payload)
            except HostedError as error:
                # A definitive client rejection created no session. Preserve that fact so an
                # operator can retry the same job after correcting configuration. Unknown
                # transport/5xx outcomes remain blocked against duplicate paid submissions.
                if error.status_code in (400,401,403,404,422):
                    self.persist(path,{'rejected':True,'error_code':str(error),'model':model,'started':time.time()})
                raise
            session_id=response['id']
            state={**state,'session_id':session_id,'model':model,'started':time.time(),'submitting':False}
            state.pop('restart_after_terminal_failure',None)
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
            turns=self.request('GET',f'/agents/sessions/{session_id}/turns',params={'limit':100,'order':'desc'}).get('data',[])
            current_usage=session.get('usage') or sum_usage([t['usage'] for t in turns if t.get('usage') is not None])
            previous=state.get('prior_sessions',[])
            usage=sum_usage([p['usage'] for p in previous if p.get('usage') is not None]+([current_usage] if current_usage is not None else []))
            state['turn_usage_incomplete']=any(p.get('usage') is None or p.get('unknown_usage') for p in previous) or any(t.get('usage') is None for t in turns)
            if usage is not None:
                # Session usage may be null even when completed turns expose usage.
                # Use cumulative turn usage then, and price cached input separately.
                usd=sum(usage_cost(p['usage'],p.get('model',model),True) for p in previous if p.get('usage') is not None)+(usage_cost(current_usage,model,True) if current_usage is not None else 0)
                standard=sum(usage_cost(p['usage'],p.get('model',model)) for p in previous if p.get('usage') is not None)+(usage_cost(current_usage,model) if current_usage is not None else 0)
                state.update(usage=usage,estimated_usd=usd,standard_rate_estimate_usd=standard)
                self.persist(path,state)
                if usd>=budget:
                    self.cancel(session_id); raise HostedError('phase_budget_threshold_reached')
            failed=turns and turns[0]['status']=='failed'
            retryable=failed and (turns[0].get('error') or {}).get('code') in TRANSIENT_TURN_ERRORS
            if failed and not retryable:
                raise HostedError('remote_turn_failed')
            if retryable and session.get('status')!='failed':
                # Session and turn reads can straddle a transition to terminal failure.
                session=self.request('GET',f'/agents/sessions/{session_id}')
            if retryable and session.get('status')=='failed':
                # A terminal managed session cannot accept another turn. Recreate ONLY
                # this failed phase, keeping its prior usage and IDs for accounting.
                # Translations from a different completed phase remain untouched.
                if len(previous)>=2:
                    raise HostedError('session_recreation_limit_exhausted')
                archived={**state,'current_session_usage':current_usage}
                self.persist(path.with_name(path.stem+f'.failed-{len(previous)+1}.json'),archived)
                prior=previous+[{'session_id':session_id,'usage':current_usage,'unknown_usage':any(t.get('usage') is None for t in turns),'started':state.get('started'),'model':state.get('model',model)}]
                self.persist(path,{'prior_sessions':prior,'restart_after_terminal_failure':True,'model':model})
                return self.phase(job,phase,schema,extra,store,budget)

            if turns and (turns[0]['status']=='cancelled' or retryable):
                # Resume cancellation or a transient failed turn in the SAME session and allocation.
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
                state['recovered_tool_errors']=successful_reads(items,turns[0]['id'],job['lesson_id'])
                for item in items:
                    if item.get('turn_id')!=turns[0]['id']:
                        continue
                    if item.get('role')=='assistant' and item.get('phase')=='final_answer':
                        for part in item.get('content',[]):
                            if part.get('type')=='output_text': text_parts.append(part['text'])
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
