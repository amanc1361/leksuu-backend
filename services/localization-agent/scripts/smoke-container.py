#!/usr/bin/env python3
import os
import secrets
import subprocess
import time
name='localization-smoke-'+secrets.token_hex(4)
env={**os.environ,'MCP_BEARER_TOKEN':secrets.token_urlsafe(40)}
try:
    subprocess.run(['docker','run','-d','--rm','--name',name,'--network=none','--read-only','--tmpfs','/tmp','--cap-drop','ALL','--security-opt','no-new-privileges:true','-e','MCP_BEARER_TOKEN','-e','LOCALIZATION_DATABASE_URL=postgresql://restricted@db.invalid/test','-e','MCP_ALLOWED_HOST=localhost','leksuu-localization:test'],env=env,check=True,stdout=subprocess.DEVNULL)
    code="""
import httpx,os
from mcp.types import LATEST_PROTOCOL_VERSION
with httpx.Client(base_url='http://localhost:8000',trust_env=False) as c:
 assert c.get('/healthz').json()['status']=='ok'
 assert c.post('/mcp',json={}).status_code==401
 h={'Authorization':'Bearer '+os.environ['MCP_BEARER_TOKEN'],'Accept':'application/json, text/event-stream','MCP-Protocol-Version':LATEST_PROTOCOL_VERSION}
 r=c.post('/mcp',headers=h,json={'jsonrpc':'2.0','id':1,'method':'tools/call','params':{'name':'get_rules','arguments':{}}})
 assert r.status_code==200 and not r.json()['result'].get('isError')
 print('Non-root read-only container: health, auth, MCP rules passed without external network')
"""
    for _ in range(30):
        p=subprocess.run(['docker','exec',name,'/app/.venv/bin/python','-c',code],capture_output=True,text=True)
        if p.returncode==0:
            print(p.stdout.strip());break
        time.sleep(.2)
    else: raise SystemExit('Container smoke failed; inspect local container before cleanup')
finally:
    subprocess.run(['docker','rm','-f',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
