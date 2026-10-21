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
            assert payload['agent']['tools'][0]['connection_origin']=='service'
            assert payload['agent']['tools'][0]['allowed_tools']==['get_lesson_context','get_rules']
            assert not payload['agent']['multi_agent']['enabled']
            return httpx.Response(201,json={'id':'sess_test'})
        if req.url.path.endswith('/turns'): return httpx.Response(200,json={'data':[{'id':'turn1','status':'completed'}]})
        if req.url.path.endswith('/items'):
            return httpx.Response(200,json={'data':[{'type':'mcp_call','name':n,'status':'completed','turn_id':'turn1'} for n in ('get_rules','get_lesson_context')]+[{'type':'message','role':'assistant','phase':'final_answer','turn_id':'turn1','content':[{'type':'output_text','text':'{"result":"ok"}'}]}],'has_more':False})
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


def test_cancelled_phase_resumes_same_session_with_idempotency_key(tmp_path,monkeypatch):
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
            return httpx.Response(200,json={'data':[{'id':'turn1' if resumed else 'turn0','status':'completed' if resumed else 'cancelled'}]})
        if req.url.path.endswith('/items'):
            return httpx.Response(200,json={'data':[{'type':'mcp_call','name':n,'status':'completed','turn_id':'turn1'} for n in ('get_rules','get_lesson_context')]+[{'type':'message','role':'assistant','phase':'final_answer','turn_id':'turn1','content':[{'type':'output_text','text':'{"resumed":true}'}]}],'has_more':False})
        return httpx.Response(200,json={'status':'idle','usage':{'input_tokens':10,'output_tokens':10}})
    class FakeStore:
        def heartbeat(self,*args): pass
        def status(self,*args): return {'run':{'status':'running'}}
    job=dict(id=uuid4(),claim_token=uuid4(),run_id=uuid4(),lesson_id=uuid4(),locale='fa',source_hash='a'*64,model='gpt-6.1-sol',review_model='gpt-6.1-sol',session_id='sess_existing')
    h=Hosted(httpx.Client(base_url='https://api.openai.com/v1',transport=httpx.MockTransport(handler)),tmp_path)
    assert h.phase(job,'translate',{'type':'object'},{},FakeStore(),1)=={'resumed':True}
    assert len(keys)==1
