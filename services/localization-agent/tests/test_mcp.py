import json
from starlette.testclient import TestClient
from mcp.types import LATEST_PROTOCOL_VERSION
from localizer.server import Guard,create_server

TOKEN='x'*40


def test_real_mcp_handshake_tools_auth_and_limits(store):
    mcp=create_server(store)
    app=Guard(mcp.streamable_http_app(),TOKEN)
    headers={'Authorization':'Bearer '+TOKEN,'Accept':'application/json, text/event-stream','Content-Type':'application/json','MCP-Protocol-Version':LATEST_PROTOCOL_VERSION}
    with TestClient(app,base_url='http://localhost') as client:
        assert client.post('/mcp',json={}).status_code==401
        assert client.post('/mcp',headers={**headers,'Content-Length':'262145'},content='x').status_code==413
        initialized=client.post('/mcp',headers=headers,json={'jsonrpc':'2.0','id':1,'method':'initialize','params':{'protocolVersion':LATEST_PROTOCOL_VERSION,'capabilities':{},'clientInfo':{'name':'integration','version':'1'}}})
        assert initialized.status_code==200,initialized.text
        result=client.post('/mcp',headers=headers,json={'jsonrpc':'2.0','id':2,'method':'tools/list','params':{}}).json()
        tools={t['name'] for t in result['result']['tools']}
        assert tools=={'list_lessons','get_lesson_context','get_rules','get_run_status','save_reviewed_draft'}
        assert not any('sql' in t for t in tools)
        lesson=str(store.lessons(limit=1)[0]['id'])
        response=client.post('/mcp',headers=headers,json={'jsonrpc':'2.0','id':3,'method':'tools/call','params':{'name':'get_lesson_context','arguments':{'lesson_id':lesson}}}).json()
        assert not response['result'].get('isError')
        body=json.loads(response['result']['content'][0]['text'])
        assert body['fields'] and body['context']['exercises']
        bad=client.post('/mcp',headers=headers,json={'jsonrpc':'2.0','id':4,'method':'tools/call','params':{'name':'get_lesson_context','arguments':{'lesson_id':"'; DROP TABLE exercises; --"}}}).json()
        assert bad['result']['isError']


def test_tool_errors_do_not_expose_database_or_content(monkeypatch,store):
    def fail(*args): raise RuntimeError('CANARY_SENSITIVE_CONTENT')
    monkeypatch.setattr(store,'context',fail)
    app=Guard(create_server(store).streamable_http_app(),TOKEN)
    headers={'Authorization':'Bearer '+TOKEN,'Accept':'application/json, text/event-stream','MCP-Protocol-Version':LATEST_PROTOCOL_VERSION}
    with TestClient(app,base_url='http://localhost') as client:
        r=client.post('/mcp',headers=headers,json={'jsonrpc':'2.0','id':1,'method':'tools/call','params':{'name':'get_lesson_context','arguments':{'lesson_id':'00000000-0000-0000-0000-000000000001'}}})
        assert r.json()['result']['isError']
        assert 'CANARY_SENSITIVE_CONTENT' not in r.text
