import copy
import json
from pathlib import Path
from uuid import uuid4

import pytest
from starlette.testclient import TestClient

from localizer.admin_providers import Providers
from localizer.admin_server import create_app
from localizer.admin_store import Workspace
from localizer.admin_worker import Worker
from localizer.content import plan
from localizer.hosted import HostedError


@pytest.fixture
def workspace(store):
    with store.connect() as c:
        c.execute('DROP SCHEMA IF EXISTS localization_admin CASCADE')
        c.execute('ALTER TABLE public.units ADD COLUMN IF NOT EXISTS position integer DEFAULT 1')
        c.execute('ALTER TABLE public.lessons ADD COLUMN IF NOT EXISTS position integer DEFAULT 1')
        c.execute('CREATE TABLE public.content_translations(source_hash text,locale text,source_lang text,source_text text,text text,origin text,updated_at timestamptz DEFAULT now(),PRIMARY KEY(source_hash,locale))')
        c.execute((Path(__file__).parents[1]/'sql/004_admin_workspace.sql').read_text())
    return Workspace(store.dsn)


@pytest.fixture
def provider():
    return {'id':str(uuid4()),'model':'test-model','kind':'gemini','name':'test',
            'base_url':'https://example.com/v1beta','api_key':'never-return-this-key',
            'input_rate':2,'output_rate':10}


def field_of(workspace,context,kind='lesson'):
    detail=workspace.lesson(context['lesson']['id'],'fa')
    return next(f for f in detail['fields'] if f['source_type']==kind)


def save(workspace,context,field,text='ترجمهٔ آزمایشی'):
    return workspace.manual(context['lesson']['id'],'fa',field['key'],field['source_hash'],text,field['revision'],'admin-test')


def test_field_manual_and_publication_preserve_all_original_content(workspace,context):
    lesson_id=context['lesson']['id']
    before=workspace.context(lesson_id)
    field=field_of(workspace,context)
    draft=save(workspace,context,field)
    updated=next(f for f in draft['fields'] if f['key']==field['key'])
    assert updated['status']=='draft'
    assert workspace.context(lesson_id)==before
    result=workspace.publish(lesson_id,'fa',[field['key']],{field['key']:updated['revision']},'admin-test')
    assert next(f for f in result['fields'] if f['key']==field['key'])['status']=='published'
    after=workspace.context(lesson_id)
    assert after['lesson']['title']==before['lesson']['title']
    assert after['items']==before['items'] and after['exercises']==before['exercises']
    assert after['lesson']['objective']['title']['en']==before['lesson']['title']
    assert after['lesson']['objective']['title']['fa']=='ترجمهٔ آزمایشی'
    with workspace.connect() as c:
        assert c.execute("SELECT count(*) n FROM localization_admin.audit WHERE action='publish'").fetchone()['n']==1


def test_duplicate_queue_and_manual_drafts_are_skipped(workspace,context,provider):
    lesson_id=context['lesson']['id']; field=field_of(workspace,context)
    save(workspace,context,field)
    first=workspace.enqueue([lesson_id],'fa',provider,5,[],False,'admin')
    second=workspace.enqueue([lesson_id],'fa',provider,5,[],False,'admin')
    assert first['queued']==len(plan(context)['fields'])-1
    assert second['queued']==0
    with workspace.connect() as c:
        assert c.execute('SELECT count(*) n FROM localization_admin.tasks').fetchone()['n']==first['queued']


def test_stale_source_and_conflicting_edits_rejected(workspace,context):
    lesson_id=context['lesson']['id']; field=field_of(workspace,context)
    save(workspace,context,field)
    with pytest.raises(ValueError,match='draft_changed_reload'):
        save(workspace,context,field)
    with workspace.connect() as c:
        c.execute("UPDATE public.lessons SET title='Changed source' WHERE id=%s",(lesson_id,))
    with pytest.raises(ValueError,match='source_changed_reload'):
        save(workspace,context,field)
    current=workspace.lesson(lesson_id,'fa')['fields'][0]
    assert current['stale_previous'] and current['status']=='missing'


def test_published_existing_content_is_counted_without_retranslation(workspace,context,provider):
    lesson_id=context['lesson']['id']
    field=field_of(workspace,context,'lesson_item')
    from localizer.publication import dictionary_hash
    with workspace.connect() as c:
        c.execute("INSERT INTO public.content_translations VALUES (%s,'fa','en',%s,'ترجمهٔ موجود','machine')",(dictionary_hash(field['text']),field['text']))
    result=workspace.enqueue([lesson_id],'fa',provider,5,[field['key']],False,'admin')
    assert result['queued']==0 and result['skipped']==1
    assert next(f for f in workspace.lesson(lesson_id,'fa')['fields'] if f['key']==field['key'])['status']=='published'


def test_legacy_editorial_publication_is_shown_until_a_new_manual_edit(workspace,context):
    from localizer.publication import dictionary_hash
    lesson_id=context['lesson']['id'];field=field_of(workspace,context,'lesson_item')
    with workspace.connect() as c:
        row=workspace.ensure(c,lesson_id,'fa',field)
        c.execute("UPDATE localization_admin.segments SET translation='پیش‌نویس قدیمی',legacy_imported=true,revision=1,status='draft' WHERE id=%s",(row['id'],))
        c.execute("INSERT INTO public.content_translations VALUES (%s,'fa','en',%s,'ترجمهٔ منتشرشدهٔ اصلاح‌شده','machine')",(dictionary_hash(field['text']),field['text']))
    current=next(f for f in workspace.lesson(lesson_id,'fa')['fields'] if f['key']==field['key'])
    assert current['status']=='published' and current['translation']=='ترجمهٔ منتشرشدهٔ اصلاح‌شده'
    updated=save(workspace,context,current,'پیش‌نویس جدید دستی')
    current=next(f for f in updated['fields'] if f['key']==field['key'])
    assert current['status']=='draft' and current['translation']=='پیش‌نویس جدید دستی'


def test_catalog_counts_live_fields_and_inline_publication(workspace,context):
    field=field_of(workspace,context)
    draft=save(workspace,context,field)
    row=next(f for f in draft['fields'] if f['key']==field['key'])
    workspace.publish(context['lesson']['id'],'fa',[field['key']],{field['key']:row['revision']},'admin')
    result=workspace.catalog(context['course']['id'],context['unit']['id'],'fa')
    assert result['lessons'][0]['total']==len(plan(context)['fields'])
    assert result['lessons'][0]['counts']['published']==1
    assert result['lessons'][0]['counts']['missing']==len(plan(context)['fields'])-1


def test_individual_exercise_publication_preserves_answer_and_options(workspace,context):
    field=field_of(workspace,context,'exercise')
    before=workspace.context(context['lesson']['id'])
    draft=save(workspace,context,field)
    row=next(f for f in draft['fields'] if f['key']==field['key'])
    workspace.publish(context['lesson']['id'],'fa',[field['key']],{field['key']:row['revision']},'admin')
    after=workspace.context(context['lesson']['id'])
    published=next(f for f in workspace.lesson(context['lesson']['id'],'fa')['fields'] if f['key']==field['key'])
    assert published['id']==row['id'] and published['status']=='published'
    for old,new in zip(before['exercises'],after['exercises']):
        assert old['answer_key']==new['answer_key']
        for key,value in old['prompt'].items():
            if key in ('instruction','explanation') and old['id']==field['source_id']:
                if isinstance(new['prompt'][key],dict):
                    assert new['prompt'][key]['en']==value or isinstance(value,dict)
            else:
                assert value==new['prompt'][key]


def test_cancel_pending_releases_fields_for_new_run(workspace,context,provider):
    lesson_id=context['lesson']['id']
    first=workspace.enqueue([lesson_id],'fa',provider,5,[],False,'admin')
    workspace.control(first['run_id'],'cancelled','admin')
    second=workspace.enqueue([lesson_id],'fa',provider,5,[],False,'admin')
    assert second['queued']==first['queued']


def test_worker_restart_marks_unknown_and_pauses_run(workspace,context,provider):
    first=workspace.enqueue([context['lesson']['id']],'fa',provider,5,[],False,'admin')
    worker=Worker(workspace,object()); task=worker.claim()
    worker.recover()
    with workspace.connect() as c:
        assert c.execute('SELECT status FROM localization_admin.tasks WHERE id=%s',(task['id'],)).fetchone()['status']=='unknown'
        assert c.execute('SELECT status FROM localization_admin.runs WHERE id=%s',(first['run_id'],)).fetchone()['status']=='paused'


def test_shared_translation_replacement_requires_explicit_confirmation(workspace,context):
    from localizer.publication import dictionary_hash
    field=field_of(workspace,context,'lesson_item')
    with workspace.connect() as c:
        c.execute("INSERT INTO public.content_translations VALUES (%s,'fa','en',%s,'ترجمهٔ موجود','machine')",(dictionary_hash(field['text']),field['text']))
    draft=save(workspace,context,field)
    row=next(f for f in draft['fields'] if f['key']==field['key'])
    with pytest.raises(ValueError,match='shared_translation_conflict'):
        workspace.publish(context['lesson']['id'],'fa',[field['key']],{field['key']:row['revision']},'admin')
    workspace.publish(context['lesson']['id'],'fa',[field['key']],{field['key']:row['revision']},'admin',True)


def fake_result(instruction,task,schema):
    field=task['fields'][0]
    if 'candidate' in task:
        return {'findings':[dict(key=field['key'],meaning='pass',completeness='pass',terminology='pass',language='pass',pedagogy='pass',issues=[])]}
    return {'locale':task['locale'],'context_hash':task['context_hash'],'translations':[
        {'key':field['key'],'text':'ترجمهٔ هوش مصنوعی','supplementary_note':'','issues':[]}]}


def test_worker_saves_reviewed_draft_and_preserves_manual_race(workspace,context,provider,monkeypatch):
    field=field_of(workspace,context); lesson_id=context['lesson']['id']
    class Config:
        def get(self,identity): return provider
    calls=[]
    def generate(p,instruction,task,schema,max_output):
        calls.append(task)
        return fake_result(instruction,task,schema),{'input_tokens':10,'output_tokens':10,'incomplete':False}
    monkeypatch.setattr('localizer.admin_worker.call',generate)
    workspace.enqueue([lesson_id],'fa',provider,5,[field['key']],False,'admin')
    worker=Worker(workspace,Config()); worker.process(worker.claim())
    assert len(calls)==2
    detail=workspace.lesson(lesson_id,'fa'); new=next(f for f in detail['fields'] if f['key']==field['key'])
    assert new['status']=='draft' and new['translation']=='ترجمهٔ هوش مصنوعی'
    workspace.enqueue([lesson_id],'fa',provider,5,[field['key']],True,'admin')
    task=worker.claim()
    save(workspace,context,new,'ویرایش دستی محفوظ')
    worker.process(task)
    assert next(f for f in workspace.lesson(lesson_id,'fa')['fields'] if f['key']==field['key'])['translation']=='ویرایش دستی محفوظ'


def test_unknown_outcome_pauses_run_and_is_not_auto_requeued(workspace,context,provider,monkeypatch):
    class Config:
        def get(self,identity): return provider
    def fail(*args): raise HostedError('provider_outcome_unknown')
    monkeypatch.setattr('localizer.admin_worker.call',fail)
    run=workspace.enqueue([context['lesson']['id']],'fa',provider,5,[],False,'admin')
    worker=Worker(workspace,Config()); worker.process(worker.claim())
    assert worker.claim() is None
    with workspace.connect() as c:
        row=c.execute('SELECT * FROM localization_admin.runs WHERE id=%s',(run['run_id'],)).fetchone()
        assert row['status']=='paused' and row['reserved_usd']>0
        assert c.execute("SELECT count(*) n FROM localization_admin.tasks WHERE status='unknown'").fetchone()['n']==1


def test_budget_exhaustion_makes_no_inference(workspace,context,provider,monkeypatch):
    class Config:
        def get(self,identity): return provider
    def unexpected(*args): pytest.fail('must reserve before inference')
    monkeypatch.setattr('localizer.admin_worker.call',unexpected)
    workspace.enqueue([context['lesson']['id']],'fa',provider,0.000001,[],False,'admin')
    worker=Worker(workspace,Config()); worker.process(worker.claim())
    assert workspace.runs()[0]['status']=='paused'


def test_provider_keys_are_redacted_and_preserved_on_edit(tmp_path,monkeypatch):
    monkeypatch.setattr('localizer.admin_providers.socket.getaddrinfo',lambda *a,**k:[(0,0,0,'',('8.8.8.8',443))])
    config=Providers(tmp_path)
    data={'name':'Example','kind':'openai','model':'test','api_key':'SECRET','input_rate':2,'output_rate':10}
    result=config.save(data)
    assert result['has_key'] and 'api_key' not in result
    changed=config.save({**data,'id':result['id'],'api_key':'','model':'changed'})
    assert config.get(changed['id'])['api_key']=='SECRET'
    assert 'SECRET' not in json.dumps(config.public())
    assert next(tmp_path.glob('*.json')).stat().st_mode & 0o777==0o600


def test_provider_internal_endpoints_rejected(tmp_path,monkeypatch):
    monkeypatch.setattr('localizer.admin_providers.socket.getaddrinfo',lambda *a,**k:[(0,0,0,'',('127.0.0.1',443))])
    with pytest.raises(ValueError,match='public_https_endpoint_required'):
        Providers(tmp_path).save({'kind':'compatible','base_url':'https://localhost','model':'test','api_key':'SECRET','input_rate':2,'output_rate':10})


def test_internal_api_requires_service_token_and_actor(tmp_path,monkeypatch):
    monkeypatch.setenv('LOCALIZATION_ADMIN_TOKEN','x'*40)
    app=create_app(store=object(),providers=Providers(tmp_path),start_worker=False)
    with TestClient(app) as client:
        assert client.get('/providers').status_code==401
        assert client.get('/providers',headers={'Authorization':'Bearer '+'x'*40}).status_code==403
        assert client.get('/providers',headers={'Authorization':'Bearer '+'x'*40,'X-Localization-Actor':'admin'}).json()=={'providers':[]}


def test_compatible_provider_uses_portable_json_mode(provider,monkeypatch):
    import httpx
    from localizer.admin_providers import call
    provider={**provider,'kind':'compatible'}
    def handler(request):
        body=json.loads(request.content)
        assert body['max_tokens']==4096 and 'max_completion_tokens' not in body
        assert body['response_format']=={'type':'json_object'}
        assert 'Return JSON matching this schema:' in body['messages'][0]['content']
        return httpx.Response(200,json={'choices':[{'finish_reason':'stop','message':{'content':'{"ok":true}'}}],'usage':{'prompt_tokens':5,'completion_tokens':3}})
    client=httpx.Client(transport=httpx.MockTransport(handler))
    monkeypatch.setattr('localizer.admin_providers.endpoint',lambda value:value)
    monkeypatch.setattr('localizer.admin_providers.httpx.Client',lambda **kwargs:client)
    output,usage=call(provider,'instruction',{}, {'type':'object'})
    assert output=={'ok':True} and usage['input_tokens']==5


def test_non_english_mapper_uses_course_source_language(context):
    context=copy.deepcopy(context); context['course']['language']='de'
    context['lesson']['objective']={'summary':{'de':'Zusammenfassung','en':'Not the source'}}
    mapped=plan(context)
    assert next(f for f in mapped['fields'] if f['path']=='objective.summary.de')['text']=='Zusammenfassung'


def test_model_catalog_reuses_connection_and_rotates_shared_key(tmp_path, monkeypatch):
    monkeypatch.setattr('localizer.admin_providers.endpoint', lambda value: value)
    config = Providers(tmp_path)
    root = config.save(dict(name='OpenAI', kind='openai', model='model-a', api_key='SECRET', input_rate=2, output_rate=10))
    child = config.save(dict(connection_id=root['id'], name='Economical', model='model-b', input_rate=.1, output_rate=.4, notes='Quick drafts', kind='compatible', base_url='https://wrong.example', api_key='IGNORED'))
    assert child['connection_id'] == root['id']
    assert child['kind'] == 'openai' and child['base_url'] == root['base_url']
    assert child['notes'] == 'Quick drafts' and child['input_rate'] == .1
    assert config.get(child['id'])['api_key'] == 'SECRET'
    stored = json.loads((tmp_path / (child['id'] + '.json')).read_text())
    assert 'api_key' not in stored
    config.save({**root, 'api_key': 'ROTATED'})
    assert config.get(child['id'])['api_key'] == 'ROTATED'
    edited = config.save({**child, 'output_rate': .8, 'api_key': ''})
    assert edited['output_rate'] == .8
    assert 'ROTATED' not in json.dumps(config.public())
    assert 'SECRET' not in json.dumps(config.public())
    assert config.get(root['id'])['model'] == 'model-a'
    with pytest.raises(ValueError, match='invalid_model_connection'):
        config.save(dict(connection_id=child['id'], model='model-c', input_rate=1, output_rate=2))
    with pytest.raises(ValueError, match='invalid_model_connection'):
        config.save({**root, 'connection_id': child['id']})
