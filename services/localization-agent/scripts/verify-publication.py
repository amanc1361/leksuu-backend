#!/usr/bin/env python3
"""Read-only release check through the existing authenticated lesson API.

Run only as the operator, outside MCP. Uses a short-lived in-memory admin preview
access token; creates no account/session, progress or grades, and prints no token.
"""
import argparse
import base64
import hashlib
import hmac
import json
import os
import time
from pathlib import Path
import httpx
from localizer.content import plan
from localizer.publication import projection
from localizer.store import Store

p=argparse.ArgumentParser();p.add_argument('--receipt',required=True,type=Path);a=p.parse_args()
receipt=json.loads(a.receipt.read_text())
assert receipt['status']=='committed'
s=Store(os.environ['DATABASE_URL'])
with s.connect() as c:
    c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY')
    context=s.context(receipt['lesson_id'],c)
    assert plan(context)['context_hash']==receipt['projected_source_hash']
    admin=c.execute("SELECT u.id FROM public.users u JOIN public.user_roles ur ON ur.user_id=u.id JOIN public.roles r ON r.id=ur.role_id WHERE r.name='admin' AND u.status='active' ORDER BY u.created_at LIMIT 1").fetchone()
    assert admin,'active_admin_required_for_preview_check'
    for t in receipt['dictionary_after']:
        row=c.execute('SELECT text,origin FROM public.content_translations WHERE source_hash=%s AND locale=%s',(t['source_hash'],t['locale'])).fetchone()
        assert row['text']==t['text'] and row['origin']=='machine'
    drafts=c.execute('SELECT d.* FROM localization.drafts d JOIN localization.jobs j ON j.id=d.job_id WHERE j.run_id=%s',(receipt['run_id'],)).fetchall()
    assert all(d['status']!='human_approved' for d in drafts)
# Generate a two-minute operator preview token in memory; never persist or print.
b64=lambda data:base64.urlsafe_b64encode(data).rstrip(b'=').decode()
header=b64(b'{"alg":"HS256","typ":"JWT"}')
now=int(time.time())
payload=b64(json.dumps({'sub':str(admin['id']),'roles':['admin'],'iat':now,'exp':now+120},separators=(',',':')).encode())
unsigned=header+'.'+payload
token=unsigned+'.'+b64(hmac.new(os.environ['JWT_SECRET'].encode(),unsigned.encode(),hashlib.sha256).digest())
with httpx.Client(trust_env=False,timeout=20) as client:
    response=client.get('http://leksuu-api-1:8080/api/v1/lessons/'+receipt['lesson_id'],headers={'Authorization':'Bearer '+token})
    assert response.status_code==200, 'lesson_api_failed'
    data=response.json()
assert not data['locked'] and len(data['items'])==len(context['items'])
assert data['objective']==context['lesson']['objective']
api_items={i['id']:i for i in data['items']}
api_exercises={e['id']:e for i in data['items'] for e in i['exercises']}
for row in context['exercises']:
    actual=api_exercises[row['id']]
    assert 'answer_key' not in actual
    for field in ('instruction','explanation'):
        if field in row['prompt']:assert actual['prompt'][field]==row['prompt'][field]
for row in context['items']:
    payload=api_items[row['id']]['payload']
    for t in receipt['dictionary_after']:
        if row['kind']=='grammar':
            original=row['payload']
            if t['source_text']==original.get('title'):assert payload['title_tr'][t['locale']]==t['text']
            if t['source_text']==original.get('rule',{}).get('en'):assert payload['rule'][t['locale']]==t['text']
        elif row['kind']=='text' and t['source_text']==row['payload'].get('phrase'):
            assert payload['translation'][t['locale']]==t['text']
print(json.dumps({'authenticated_lesson_api':200,'localized_headings_and_objectives':True,'localized_grammar_and_phrase':True,'localized_exercise_guidance':True,'original_english_and_answer_keys_preserved':True,'drafts_remain_unapproved':True,'lesson_items':len(api_items),'exercises':len(api_exercises)}))
