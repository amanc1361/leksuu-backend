#!/usr/bin/env python3
"""Read-only verification through real auth/admin APIs; never prints credentials."""
import argparse
import base64
import hashlib
import hmac
import json
import os
from pathlib import Path
import time

import httpx
from localizer.store import Store

p=argparse.ArgumentParser()
p.add_argument('--report',type=Path,required=True)
p.add_argument('--session-file',type=Path)
a=p.parse_args()
os.umask(0o077)
store=Store(os.environ['DATABASE_URL'])
workspace=Store(os.environ['LOCALIZATION_ADMIN_DATABASE_URL'])
with store.connect() as c:
    c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY')
    users={}
    for role in ('admin','student','teacher'):
        user=c.execute("SELECT u.id FROM public.users u JOIN public.user_roles ur ON ur.user_id=u.id JOIN public.roles r ON r.id=ur.role_id WHERE r.name=%s AND u.status='active' ORDER BY u.created_at LIMIT 1",(role,)).fetchone()
        if user:
            users[role]=str(user['id'])
    assert 'admin' in users
    privileges=c.execute("SELECT has_table_privilege('localization_admin_service','public.users','SELECT') users_read,has_column_privilege('localization_admin_service','public.exercises','answer_key','UPDATE') answers_write,has_table_privilege('localization_admin_service','public.lessons','DELETE') lesson_delete").fetchone()
    assert not any(privileges.values())
with workspace.connect() as c:
    baseline={name:c.execute('SELECT count(*) n FROM localization_admin.'+name).fetchone()['n'] for name in ('runs','tasks')}


def token(role):
    b64=lambda data:base64.urlsafe_b64encode(data).rstrip(b'=').decode()
    now=int(time.time())
    value=b64(b'{"alg":"HS256","typ":"JWT"}')+'.'+b64(json.dumps({'sub':users[role],'roles':[role],'iat':now,'exp':now+600},separators=(',',':')).encode())
    return value+'.'+b64(hmac.new(os.environ['JWT_SECRET'].encode(),value.encode(),hashlib.sha256).digest())


with httpx.Client(base_url='http://leksuu-api-1:8080/api/v1/admin/localization',trust_env=False,timeout=35) as client:
    denied={'anonymous':client.get('/catalog').status_code}
    assert denied['anonymous']==401
    for role in ('student','teacher'):
        if role in users and users[role]!=users['admin']:
            denied[role]=client.get('/catalog',headers={'Authorization':'Bearer '+token(role)}).status_code
            assert denied[role]==403
    access=token('admin')
    headers={'Authorization':'Bearer '+access}
    result=client.get('/catalog',headers=headers)
    assert result.status_code==200
    catalog=result.json()
    providers=client.get('/providers',headers=headers)
    assert providers.status_code==200
    configs=providers.json()['providers']
    assert all('api_key' not in item for item in configs)
    start=time.monotonic()
    chapter=client.get('/catalog',params={'course_id':'76d38137-3246-432d-b2d1-e5dbc87481b8','unit_id':'9ad02a5c-5971-4f1a-b8eb-c4927fccb363','locale':'fa'},headers=headers)
    assert chapter.status_code==200
    chapter=chapter.json()
    assert len(chapter['lessons'])==7
    assert sum(l['total'] for l in chapter['lessons'])==58
    elapsed=round(time.monotonic()-start,3)
    pilot={}
    for locale in ('fa','ckb'):
        detail=client.get('/lessons/0625767d-231d-4c45-a340-c389dce5ec2a',params={'locale':locale},headers=headers)
        assert detail.status_code==200
        fields=detail.json()['fields']
        pilot[locale]={'fields':len(fields),'published':sum(f['status']=='published' for f in fields)}
        assert pilot[locale]=={'fields':17,'published':17}
    assert client.get('/runs',headers=headers).status_code==200
    memory_preview=client.post('/queue/preview',headers=headers,json={'lesson_ids':['bc33993c-741f-4617-84f8-00964aca89c0'],'locale':'fa','keys':[],'force':False})
    assert memory_preview.status_code==200
    memory_preview=memory_preview.json()
    assert sum(memory_preview[k] for k in ('existing','active','reusable','waiting','fresh','review'))==memory_preview['total']
    publication_preview=client.post('/publish/preview',headers=headers,json={'lesson_ids':['bc33993c-741f-4617-84f8-00964aca89c0'],'locale':'fa'})
    assert publication_preview.status_code==200
    # Preview stores only private approval metadata. Never confirm real lesson drafts here.
    publication_preview=publication_preview.json()
    assert publication_preview['batch_id'] and publication_preview['locale']=='fa'

with workspace.connect() as c:
    after={name:c.execute('SELECT count(*) n FROM localization_admin.'+name).fetchone()['n'] for name in ('runs','tasks')}
assert baseline==after
report=dict(translation_preflight=memory_preview,bulk_publication_preview_http=200,publication_confirmed=False,admin_http=200,denied_roles=denied,courses=len(catalog['courses']),
    chapter1_lessons=7,chapter1_fields_per_language=58,chapter_catalog_seconds=elapsed,
    previous_pilot_coverage=pilot,provider_keys_redacted=True,
    providers=[{'kind':p['kind'],'model':p['model'],'has_key':p['has_key']} for p in configs],
    role_cannot_read_users_or_modify_answers_or_delete_lessons=True,
    provider_inference_requests=0,runs_and_tasks_unchanged=True)
a.report.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
a.report.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
if a.session_file:
    a.session_file.write_text(json.dumps({'accessToken':access,'refreshToken':'','expiresIn':600}))
print(json.dumps(report,ensure_ascii=False))
