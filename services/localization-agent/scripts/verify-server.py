#!/usr/bin/env python3
"""Run inside the deployed MCP container using its restricted role and token."""
import argparse
import json
import os
import httpx
from mcp.types import LATEST_PROTOCOL_VERSION
from localizer.content import plan
from localizer.store import Store

p=argparse.ArgumentParser();p.add_argument('--lesson',required=True);a=p.parse_args()
store=Store()
with store.connect() as c:
    user=c.execute('SELECT current_user AS name').fetchone()['name']
    assert user=='localization_service'
    for table in ('courses','units','lessons','lesson_items','exercises'):
        permissions=c.execute("SELECT has_table_privilege(current_user,%s,'SELECT') AS read,has_table_privilege(current_user,%s,'INSERT,UPDATE,DELETE,TRUNCATE') AS write",('public.'+table,'public.'+table)).fetchone()
        assert permissions['read'] and not permissions['write']
    assert not c.execute("SELECT has_column_privilege(current_user,'localization.drafts','status','UPDATE') allowed").fetchone()['allowed']
    before={name:c.execute('SELECT count(*) n FROM localization.'+name).fetchone()['n'] for name in ('runs','jobs','drafts')}
context=store.context(a.lesson)
mapping=plan(context)
headers={'Authorization':'Bearer '+os.environ['MCP_BEARER_TOKEN'],'Accept':'application/json, text/event-stream','MCP-Protocol-Version':LATEST_PROTOCOL_VERSION}
with httpx.Client(base_url='http://127.0.0.1:8000',trust_env=False,timeout=20) as client:
    assert client.get('/healthz').json()['status']=='ok'
    assert client.post('/mcp',json={}).status_code==401
    for ident,method,params in [(1,'initialize',{'protocolVersion':LATEST_PROTOCOL_VERSION,'capabilities':{},'clientInfo':{'name':'deployment-verifier','version':'1'}}),(2,'tools/list',{}),(3,'tools/call',{'name':'get_lesson_context','arguments':{'lesson_id':a.lesson}})]:
        response=client.post('/mcp',headers=headers,json={'jsonrpc':'2.0','id':ident,'method':method,'params':params})
        assert response.status_code==200
        result=response.json()['result']
        assert not result.get('isError')
        if ident==2:
            assert len(result['tools'])==5
        if ident==3:
            data=result.get('structuredContent') or json.loads(result['content'][0]['text'])
            assert data['context_hash']==mapping['context_hash']
with store.connect() as c:
    after={name:c.execute('SELECT count(*) n FROM localization.'+name).fetchone()['n'] for name in ('runs','jobs','drafts')}
assert before==after
print(json.dumps({'restricted_role':True,'source_writes_allowed':False,'human_approval_allowed':False,'mcp_handshake':True,'mcp_tools':5,'real_lesson_items':len(context['items']),'real_lesson_exercises':len(context['exercises']),'translatable_fields':len(mapping['fields']),'database_counts_before':before,'database_counts_after':after},indent=2))
