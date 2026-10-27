import httpx
import pytest
from localizer.hosted import Hosted,HostedError


def test_access_check_is_read_only_and_does_not_infer(tmp_path):
    paths=[]
    def handler(req):
        assert req.method=='GET'
        paths.append(req.url.path)
        return httpx.Response(200,json={'data':[{'id':'gpt-6.1-sol'}] if req.url.path.endswith('models') else []})
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    assert h.access()['candidate_models']==['gpt-6.1-sol']
    assert paths==['/v1/models','/v1/agents/sessions']


def test_no_local_fallback_or_create_retry(tmp_path):
    count=0
    def handler(req):
        nonlocal count
        count+=1
        return httpx.Response(403,json={'error':{'message':'sensitive message must not escape'}})
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    with pytest.raises(HostedError,match='openai_http_403'):
        h.access()
    assert count==1


def test_hosted_phase_requires_terminal_success_and_recovers_output(tmp_path,monkeypatch):
    from uuid import uuid4
    monkeypatch.setenv('MCP_PUBLIC_URL','https://example.com/mcp')
    monkeypatch.setenv('MCP_BEARER_TOKEN','z'*40)
    calls=[]
    def handler(req):
        calls.append((req.method,req.url.path))
        if req.method=='POST':
            import json
            payload=json.loads(req.content)
            assert payload['environment']=={'type':'none'}
            assert set(payload['agent']['text']['format'])=={'type','schema'}
            assert payload['agent']['text']['format']['type']=='json_schema'
            assert payload['agent']['tools'][0]['connection_origin']=='service'
            assert payload['agent']['tools'][0]['allowed_tools']==['get_lesson_context','get_rules']
            assert not payload['agent']['multi_agent']['enabled']
            return httpx.Response(201,json={'id':'sess_test'})
        if req.url.path.endswith('/turns'): return httpx.Response(200,json={'data':[{'id':'turn1','status':'completed'}]})
        if req.url.path.endswith('/items'):
            return httpx.Response(200,json={'data':[{'type':'mcp_call','name':n,'status':'completed','turn_id':'turn1','arguments':{'lesson_id':str(job['lesson_id'])} if n=='get_lesson_context' else {}} for n in ('get_rules','get_lesson_context')]+[{'type':'message','role':'assistant','phase':'final_answer','turn_id':'turn1','content':[{'type':'output_text','text':'{"result":"ok"}'}]}],'has_more':False})
        return httpx.Response(200,json={'status':'idle','usage':{'input_tokens':1,'output_tokens':1}})
    class FakeStore:
        def heartbeat(self,*args): pass
        def record_session(self,*args): pass
        def status(self,*args): return {'run':{'status':'running'}}
    job=dict(id=uuid4(),claim_token=uuid4(),run_id=uuid4(),lesson_id=uuid4(),locale='fa',source_hash='a'*64,model='gpt-6.1-sol',review_model='gpt-6.1-sol')
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    assert h.phase(job,'translate',{'type':'object'},{},FakeStore(),1)=={'result':'ok'}
    assert h.phase(job,'translate',{'type':'object'},{},FakeStore(),1)=={'result':'ok'}
    assert sum(method=='POST' for method,path in calls)==1


def test_unknown_submission_is_not_automatically_duplicated(tmp_path,monkeypatch):
    from uuid import uuid4
    monkeypatch.setenv('MCP_PUBLIC_URL','https://example.com/mcp')
    monkeypatch.setenv('MCP_BEARER_TOKEN','z'*40)
    def handler(req): raise httpx.ConnectError('secret must not escape')
    job=dict(id=uuid4(),lesson_id=uuid4(),locale='fa',source_hash='a'*64,model='gpt-6.1-sol',review_model='gpt-6.1-sol')
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    for _ in range(2):
        with pytest.raises(HostedError,match='submission_outcome_unknown'):
            h.phase(job,'translate',{'type':'object'},{},None,1)


@pytest.mark.parametrize('initial_status',['cancelled','failed'])
def test_interrupted_phase_resumes_same_session_with_idempotency_key(tmp_path,monkeypatch,initial_status):
    from uuid import uuid4
    import json
    monkeypatch.setattr('localizer.hosted.time.sleep',lambda _:None)
    resumed=False; keys=[]
    def handler(req):
        nonlocal resumed
        if req.method=='POST':
            assert req.url.path=='/v1/agents/sessions/sess_existing/events'
            assert json.loads(req.content)['events'][0]['type']=='agent.session.input.message'
            keys.append(req.headers['Idempotency-Key']);resumed=True
            return httpx.Response(200,json={})
        if req.url.path.endswith('/turns'):
            return httpx.Response(200,json={'data':[{'id':'turn1' if resumed else 'turn0','status':'completed' if resumed else initial_status,'error':{'code':'internal_error'} if not resumed and initial_status=='failed' else None}]})
        if req.url.path.endswith('/items'):
            return httpx.Response(200,json={'data':[{'type':'mcp_call','name':n,'status':'completed','turn_id':'turn1','arguments':{'lesson_id':str(job['lesson_id'])} if n=='get_lesson_context' else {}} for n in ('get_rules','get_lesson_context')]+[{'type':'message','role':'assistant','phase':'final_answer','turn_id':'turn1','content':[{'type':'output_text','text':'{"resumed":true}'}]}],'has_more':False})
        return httpx.Response(200,json={'status':'idle','usage':{'input_tokens':10,'output_tokens':10}})
    class FakeStore:
        def heartbeat(self,*args): pass
        def status(self,*args): return {'run':{'status':'running'}}
    job=dict(id=uuid4(),claim_token=uuid4(),run_id=uuid4(),lesson_id=uuid4(),locale='fa',source_hash='a'*64,model='gpt-6.1-sol',review_model='gpt-6.1-sol',session_id='sess_existing')
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    assert h.phase(job,'translate',{'type':'object'},{},FakeStore(),1)=={'resumed':True}
    assert len(keys)==1


def test_rejected_create_is_known_not_unknown_and_can_be_retried_after_operator_fix(tmp_path,monkeypatch):
    from uuid import uuid4
    import json
    monkeypatch.setenv('MCP_PUBLIC_URL','https://example.com/mcp')
    monkeypatch.setenv('MCP_BEARER_TOKEN','z'*40)
    calls=[]
    def handler(req):
        calls.append(req.method)
        return httpx.Response(400,json={'error':{'code':'unknown_parameter'}})
    job=dict(id=uuid4(),lesson_id=uuid4(),locale='fa',source_hash='a'*64,model='gpt-6.1-sol',review_model='gpt-6.1-sol')
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    for _ in range(2):
        with pytest.raises(HostedError,match='openai_http_400'):
            h.phase(job,'translate',{'type':'object'},{},None,1)
    state=json.loads((tmp_path/f"{job['id']}-translate.json").read_text())
    assert state['rejected'] and not state.get('submitting')
    assert calls==['POST','POST']


def test_completed_turn_usage_and_cache_pricing_without_session_usage():
    from localizer.hosted import sum_usage,usage_cost
    usage=sum_usage([{'input_tokens':10000,'input_tokens_details':{'cached_tokens':8000},'output_tokens':1000,'output_tokens_details':{'reasoning_tokens':200}}])
    assert usage['output_tokens_details']['reasoning_tokens']==200
    assert usage_cost(usage,'gpt-6.1-sol')==pytest.approx(.0148)
    assert usage_cost(usage,'gpt-6.1-sol',True)==pytest.approx(.0246)


def test_recovered_read_error_is_accepted_but_unrecovered_and_wrong_lesson_are_not():
    from localizer.hosted import successful_reads
    calls=[{'type':'mcp_call','turn_id':'t','name':'get_lesson_context','arguments':{'lesson_id':'target'},'status':'completed'},
           {'type':'mcp_call','turn_id':'t','name':'get_rules','status':'failed','error':{'message':'504'}},
           {'type':'mcp_call','turn_id':'t','name':'get_rules','status':'completed'}]
    assert successful_reads(calls,'t','target')==1
    with pytest.raises(HostedError,match='remote_mcp_tool_failed'):successful_reads(calls[:-1],'t','target')
    with pytest.raises(HostedError,match='remote_mcp_tool_failed'):successful_reads(calls,'t','different')


def test_terminal_failed_session_recreates_only_failed_phase_and_keeps_history(tmp_path,monkeypatch):
    from uuid import uuid4
    import json
    monkeypatch.setenv('MCP_PUBLIC_URL','https://example.com/mcp')
    monkeypatch.setenv('MCP_BEARER_TOKEN','z'*40)
    posts=[]; seen_turns=False
    def handler(req):
        nonlocal seen_turns
        if req.method=='POST':
            assert req.url.path=='/v1/agents/sessions'
            posts.append(req.url.path)
            return httpx.Response(201,json={'id':'sess_replacement'})
        terminal='sess_failed' in req.url.path
        if req.url.path.endswith('/turns'):
            seen_turns=True
            return httpx.Response(200,json={'data':[{'id':'turn_old' if terminal else 'turn_new','status':'failed' if terminal else 'completed','error':{'code':'internal_error'} if terminal else None,'usage':None if terminal else {'input_tokens':10,'output_tokens':10}}]})
        if req.url.path.endswith('/items'):
            items=[{'type':'mcp_call','turn_id':'turn_new','name':n,'status':'completed','arguments':{'lesson_id':str(job['lesson_id'])} if n=='get_lesson_context' else {}} for n in ('get_rules','get_lesson_context')]
            items.append({'type':'message','role':'assistant','phase':'final_answer','turn_id':'turn_new','content':[{'type':'output_text','text':'{"ok":true}'}]})
            return httpx.Response(200,json={'data':items,'has_more':False})
        return httpx.Response(200,json={'status':('failed' if seen_turns else 'in_progress') if terminal else 'idle','usage':None})
    class FakeStore:
        def heartbeat(self,*args):pass
        def status(self,*args):return {'run':{'status':'running'}}
        def record_session(self,*args):pass
    job=dict(id=uuid4(),claim_token=uuid4(),run_id=uuid4(),lesson_id=uuid4(),locale='fa',source_hash='a'*64,model='gpt-6.1-sol',review_model='gpt-6.1-sol',review_session_id='sess_failed')
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    assert h.phase(job,'review',{'type':'object'},{},FakeStore(),1)=={'ok':True}
    state=json.loads((tmp_path/f"{job['id']}-review.json").read_text())
    assert state['session_id']=='sess_replacement'
    assert state['prior_sessions'][0]['session_id']=='sess_failed'
    assert state['turn_usage_incomplete']
    assert len(posts)==1


def test_cached_output_usage_reconciliation_preserves_prior_model_cost_and_unknowns(tmp_path):
    from uuid import uuid4
    import json
    job=dict(id=uuid4(),model='gpt-6.1-sol',review_model='gpt-6-astra')
    usage={'input_tokens':1000,'input_tokens_details':{'cached_tokens':500},'output_tokens':1000}
    path=tmp_path/f"{job['id']}-review.json"
    path.write_text(json.dumps({'session_id':'sess_new','output':{'ok':True},'usage':None,'prior_sessions':[{'model':'gpt-6.1-sol','usage':usage},{'model':'gpt-6.1-sol','usage':None,'unknown_usage':True}]}))
    def handler(req):
        assert req.method=='GET' and req.url.path.endswith('/turns')
        return httpx.Response(200,json={'data':[{'id':'t','usage':usage,'status':'completed'}]})
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    assert h.phase(job,'review',{}, {},None,1)=={'ok':True}
    state=json.loads(path.read_text())
    assert state['usage']['input_tokens']==2000
    assert state['standard_rate_estimate_usd']==pytest.approx(.06655)
    assert state['turn_usage_incomplete']
