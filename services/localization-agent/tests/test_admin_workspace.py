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
        c.execute((Path(__file__).parents[1]/'sql/006_translation_memory.sql').read_text())
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
    lesson_id=context['lesson']['id']
    field=field_of(workspace,context)
    save(workspace,context,field)
    first=workspace.enqueue([lesson_id],'fa',provider,0.000001,[],False,'admin')
    worker=Worker(workspace,Config()); worker.process(worker.claim())
    assert workspace.runs()[0]['status']=='paused'
    assert worker.claim() is None
    with workspace.connect() as c:
        assert c.execute("SELECT count(*) n FROM localization_admin.tasks WHERE run_id=%s AND status IN ('pending','waiting','running')",(first['run_id'],)).fetchone()['n']==0
        assert c.execute("SELECT count(*) n FROM localization_admin.tasks WHERE run_id=%s AND status='cancelled'",(first['run_id'],)).fetchone()['n']==first['queued']-1
    detail=workspace.lesson(lesson_id,'fa')
    assert next(f for f in detail['fields'] if f['key']==field['key'])['translation']=='ترجمهٔ آزمایشی'
    assert not any(f['status'] in ('pending','running') for f in detail['fields'])
    preview=workspace.preview_queue([lesson_id],'fa')
    assert preview['active']==0 and preview['fresh']>0
    second=workspace.enqueue([lesson_id],'fa',provider,5,[],False,'admin')
    assert second['queued']==first['queued']


def test_manual_pause_keeps_queue_resumable(workspace,context,provider):
    run=workspace.enqueue([context['lesson']['id']],'fa',provider,5,[],False,'admin')
    workspace.control(run['run_id'],'paused','admin')
    worker=Worker(workspace,object())
    assert worker.claim() is None
    workspace.control(run['run_id'],'running','admin')
    assert str(worker.claim()['run_id'])==run['run_id']


def test_release_pending_preserves_newer_edit_and_restores_existing_draft(workspace,context,provider):
    lesson_id=context['lesson']['id']
    field=field_of(workspace,context)
    save(workspace,context,field)
    run=workspace.enqueue([lesson_id],'fa',provider,5,[field['key']],True,'admin')
    workspace.control(run['run_id'],'cancelled','admin')
    updated=next(f for f in workspace.lesson(lesson_id,'fa')['fields'] if f['key']==field['key'])
    assert updated['status']=='draft' and updated['translation']=='ترجمهٔ آزمایشی'
    run=workspace.enqueue([lesson_id],'fa',provider,5,[field['key']],True,'admin')
    with workspace.connect() as c:
        c.execute("UPDATE localization_admin.segments SET revision=revision+1,error_code='newer_state' WHERE id=%s",(updated['id'],))
    workspace.control(run['run_id'],'cancelled','admin')
    with workspace.connect() as c:
        assert c.execute('SELECT error_code FROM localization_admin.segments WHERE id=%s',(updated['id'],)).fetchone()['error_code']=='newer_state'


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


def test_run_history_identifies_legacy_lesson_and_distinguishes_draft_publication(workspace, context, provider, monkeypatch):
    field=field_of(workspace,context)
    class Config:
        def get(self, identity): return provider
    monkeypatch.setattr('localizer.admin_worker.call',lambda p,i,t,s,m: (fake_result(i,t,s),{'input_tokens':10,'output_tokens':10,'incomplete':False}))
    result=workspace.enqueue([context['lesson']['id']],'fa',provider,5,[field['key']],False,'admin')
    worker=Worker(workspace,Config());worker.process(worker.claim());worker.claim()
    # Existing production runs have no stored scope: reconstruct it from their tasks.
    with workspace.connect() as c:
        c.execute("UPDATE localization_admin.runs SET scope='[]' WHERE id=%s",(result['run_id'],))
    run=workspace.runs()[0];scope=run['lessons'][0]
    assert scope['lesson_title']==context['lesson']['title']
    assert scope['course_title']==context['course']['title']
    assert scope['unit_title']==context['unit']['title']
    assert scope['lesson_id']==context['lesson']['id']
    assert run['status']=='completed' and run['counts']=={'completed':1}
    assert run['drafts']==1 and run['published']==0
    assert 'provider_snapshot' not in run
    draft=next(f for f in workspace.lesson(context['lesson']['id'],'fa')['fields'] if f['key']==field['key'])
    workspace.publish(context['lesson']['id'],'fa',[field['key']],{field['key']:draft['revision']},'admin')
    run=workspace.runs()[0]
    assert run['drafts']==0 and run['published']==1


def test_fully_skipped_run_retains_selected_scope_without_success_claim(workspace,context,provider):
    first=workspace.enqueue([context['lesson']['id']],'fa',provider,5,[],False,'admin')
    second=workspace.enqueue([context['lesson']['id']],'fa',provider,5,[],False,'admin')
    assert second['queued']==0
    runs=workspace.runs();run=next(r for r in runs if str(r['id'])==second['run_id'])
    assert run['status']=='skipped' and run['counts']=={}
    assert run['lessons'][0]['lesson_title']==context['lesson']['title']
    assert run['lessons'][0]['fields']==0
    assert len(runs)==2


def test_run_history_surfaces_provider_errors(workspace,context,provider,monkeypatch):
    class Config:
        def get(self, identity): return provider
    def fail(*args): raise HostedError('provider_http_429')
    monkeypatch.setattr('localizer.admin_worker.call',fail)
    workspace.enqueue([context['lesson']['id']],'fa',provider,5,[],False,'admin')
    worker=Worker(workspace,Config());worker.process(worker.claim())
    run=workspace.runs()[0]
    assert run['status']=='paused'
    assert run['lessons'][0]['errors']==['provider_http_429']


def test_run_with_only_superseded_tasks_is_not_generation_success(workspace,context,provider):
    field=field_of(workspace,context)
    class Config:
        def get(self, identity): return provider
    workspace.enqueue([context['lesson']['id']],'fa',provider,5,[field['key']],False,'admin')
    worker=Worker(workspace,Config());task=worker.claim()
    save(workspace,context,field)
    worker.process(task);worker.claim()
    run=workspace.runs()[0]
    assert run['status']=='skipped' and run['counts']=={'skipped':1}
    assert run['lessons'][0]['errors']==['manual_edit_preserved']


def clone_context(workspace,context):
    """Same sources in a different chapter, with independent content IDs."""
    from psycopg import sql
    from psycopg.types.json import Jsonb
    result=copy.deepcopy(context)
    result['unit']['id']=str(uuid4());result['lesson']['id']=str(uuid4());result['lesson']['unit_id']=result['unit']['id']
    ids={}
    for item in result['items']:
        old=item['id'];item['id']=str(uuid4());item['lesson_id']=result['lesson']['id'];ids[old]=item['id']
    for exercise in result['exercises']:
        exercise['id']=str(uuid4());exercise['lesson_item_id']=ids[exercise['lesson_item_id']]
    with workspace.connect() as c:
        for table,rows in [('units',[result['unit']]),('lessons',[result['lesson']]),('lesson_items',result['items']),('exercises',result['exercises'])]:
            columns=[r['column_name'] for r in c.execute('SELECT column_name FROM information_schema.columns WHERE table_schema=%s AND table_name=%s ORDER BY ordinal_position',('public',table)).fetchall()]
            for row in rows:
                values=[Jsonb(row[k]) if isinstance(row.get(k),(dict,list)) else row.get(k,1 if k=='position' else None) for k in columns]
                c.execute(sql.SQL('INSERT INTO {} ({}) VALUES ({})').format(sql.Identifier(table),sql.SQL(',').join(map(sql.Identifier,columns)),sql.SQL(',').join(sql.Placeholder() for _ in columns)),values)
    return result


@pytest.fixture
def memory_context(workspace,context):
    result=copy.deepcopy(context)
    result['lesson']['title']='Hello!'
    with workspace.connect() as c:
        c.execute("UPDATE public.lessons SET title='Hello!' WHERE id=%s",(result['lesson']['id'],))
    return result


def test_memory_reuses_across_chapters_without_inference(workspace,memory_context,provider,monkeypatch):
    context=memory_context
    save(workspace,context,field_of(workspace,context),'ترجمهٔ مشترک')
    other=clone_context(workspace,context);target=field_of(workspace,other)
    preview=workspace.preview_queue([other['lesson']['id']],'fa',[target['key']])
    assert preview['reusable']==1 and preview['fresh']==0
    before=workspace.context(other['lesson']['id'])
    result=workspace.enqueue([other['lesson']['id']],'fa',provider,5,[target['key']],False,'admin')
    assert result['reused']==1 and result['queued']==0 and result['fresh']==0
    current=field_of(workspace,other)
    assert current['translation']=='ترجمهٔ مشترک' and current['origin']=='memory' and current['status']=='draft'
    assert current['memory_source_id'] and workspace.context(other['lesson']['id'])==before
    class Config:
        def get(self,identity): pytest.fail('Reuse must not resolve paid credentials')
    monkeypatch.setattr('localizer.admin_worker.call',lambda *a:pytest.fail('No inference'))
    assert Worker(workspace,Config()).claim() is None
    run=workspace.runs()[0]
    assert run['counts']=={'reused':1} and run['estimated_usd']==0 and run['reserved_usd']==0


def test_memory_separates_language_purpose_context_and_rules(workspace,memory_context,monkeypatch):
    context=memory_context
    from localizer.admin_memory import memory_key
    field=field_of(workspace,context);save(workspace,context,field)
    other=clone_context(workspace,context);target=field_of(workspace,other)
    assert workspace.preview_queue([other['lesson']['id']],'ckb',[target['key']])['fresh']==1
    source=workspace.context(context['lesson']['id']);key=memory_key(source,field,'fa')
    changed=copy.deepcopy(source);changed['course']['language']='fr'
    assert memory_key(changed,field,'fa')!=key
    changed=copy.deepcopy(source);changed['lesson']['title']='Another meaning'
    assert memory_key(changed,field,'fa')!=key
    assert memory_key(source,{**field,'path':'objective.summary'},'fa')!=key
    monkeypatch.setattr('localizer.admin_memory.versions',lambda:('changed-instructions','changed-glossary'))
    assert workspace.preview_queue([other['lesson']['id']],'fa',[target['key']])['fresh']==1


def test_memory_conflicts_and_review_findings_do_not_cause_paid_retry(workspace,memory_context,provider):
    context=memory_context
    save(workspace,context,field_of(workspace,context),'ترجمهٔ اول')
    second=clone_context(workspace,context);save(workspace,second,field_of(workspace,second),'ترجمهٔ متفاوت')
    third=clone_context(workspace,context);target=field_of(workspace,third)
    assert workspace.preview_queue([third['lesson']['id']],'fa',[target['key']])['review']==1
    result=workspace.enqueue([third['lesson']['id']],'fa',provider,5,[target['key']],False,'admin')
    assert result['queued']==0 and result['review']==1
    assert field_of(workspace,third)['error_code']=='translation_memory_requires_review'
    with workspace.connect() as c:
        c.execute("UPDATE localization_admin.segments SET issues='[\"reviewer_meaning_concern\"]' WHERE translation<>''")
    fourth=clone_context(workspace,context);target=field_of(workspace,fourth)
    assert workspace.preview_queue([fourth['lesson']['id']],'fa',[target['key']])['review']==1


def test_stale_memory_source_is_not_reused(workspace,memory_context):
    context=memory_context
    save(workspace,context,field_of(workspace,context))
    other=clone_context(workspace,context);target=field_of(workspace,other)
    with workspace.connect() as c:
        c.execute("UPDATE public.lessons SET title='Changed source' WHERE id=%s",(context['lesson']['id'],))
    assert workspace.preview_queue([other['lesson']['id']],'fa',[target['key']])['fresh']==1


def test_duplicate_fields_share_one_generation_and_review(workspace,memory_context,provider,monkeypatch):
    context=memory_context
    other=clone_context(workspace,context);field=field_of(workspace,context);target=field_of(workspace,other)
    first=workspace.enqueue([context['lesson']['id']],'fa',provider,5,[field['key']],False,'admin')
    assert workspace.preview_queue([other['lesson']['id']],'fa',[target['key']])['waiting']==1
    second=workspace.enqueue([other['lesson']['id']],'fa',provider,5,[target['key']],False,'admin')
    assert first['fresh']==1 and second['fresh']==0 and second['waiting']==1
    class Config:
        def get(self,identity): return provider
    calls=[]
    def generate(p,i,t,s,m): calls.append(t);return fake_result(i,t,s),{'input_tokens':10,'output_tokens':10,'incomplete':False}
    monkeypatch.setattr('localizer.admin_worker.call',generate)
    worker=Worker(workspace,Config());worker.process(worker.claim());assert worker.claim() is None
    assert len(calls)==2 and field_of(workspace,other)['origin']=='memory'
    run=next(r for r in workspace.runs() if str(r['id'])==second['run_id'])
    assert run['status']=='completed' and run['counts']=={'reused':1} and run['reserved_usd']==0 and run['estimated_usd']==0


def test_unknown_shared_result_never_submits_follower_inference(workspace,memory_context,provider,monkeypatch):
    context=memory_context
    other=clone_context(workspace,context);field=field_of(workspace,context);target=field_of(workspace,other)
    workspace.enqueue([context['lesson']['id']],'fa',provider,5,[field['key']],False,'admin')
    second=workspace.enqueue([other['lesson']['id']],'fa',provider,5,[target['key']],False,'admin')
    class Config:
        def get(self,identity): return provider
    calls=[]
    def fail(*a): calls.append(1);raise HostedError('provider_outcome_unknown')
    monkeypatch.setattr('localizer.admin_worker.call',fail)
    worker=Worker(workspace,Config());worker.process(worker.claim());assert worker.claim() is None
    assert len(calls)==1 and field_of(workspace,other)['error_code']=='shared_translation_requires_review'
    run=next(r for r in workspace.runs() if str(r['id'])==second['run_id'])
    assert run['status']=='failed' and run['estimated_usd']==0 and run['reserved_usd']==0


def test_memory_rechecked_before_submission_and_force_bypasses_it(workspace,memory_context,provider,monkeypatch):
    context=memory_context
    other=clone_context(workspace,context);field=field_of(workspace,context)
    workspace.enqueue([context['lesson']['id']],'fa',provider,5,[field['key']],False,'admin')
    save(workspace,other,field_of(workspace,other),'ترجمهٔ تازهٔ دستی')
    class Config:
        def get(self,identity): return provider
    calls=[]
    def generate(p,i,t,s,m): calls.append(t);return fake_result(i,t,s),{'input_tokens':1,'output_tokens':1,'incomplete':False}
    monkeypatch.setattr('localizer.admin_worker.call',generate)
    worker=Worker(workspace,Config());worker.process(worker.claim());worker.claim()
    assert calls==[] and field_of(workspace,context)['translation']=='ترجمهٔ تازهٔ دستی'
    assert workspace.runs()[0]['counts']=={'reused':1}
    preview=workspace.preview_queue([context['lesson']['id']],'fa',[field['key']],True)
    assert preview['fresh']==1 and preview['reusable']==0
    workspace.enqueue([context['lesson']['id']],'fa',provider,5,[field['key']],True,'admin')
    worker.process(worker.claim())
    assert len(calls)==2 and field_of(workspace,context)['origin']=='machine'


def test_bulk_publication_versioned_partial_and_idempotent(workspace,memory_context):
    context=memory_context
    other=clone_context(workspace,context)
    save(workspace,context,field_of(workspace,context),'ترجمهٔ درس اول')
    save(workspace,other,field_of(workspace,other),'ترجمهٔ درس دوم')
    preview=workspace.prepare_publication([context['lesson']['id'],other['lesson']['id']],'fa','admin')
    assert preview['fields']==2
    save(workspace,context,field_of(workspace,context),'ویرایش بعد از پیش‌نمایش')
    with pytest.raises(ValueError,match='publication_batch_not_found'):
        workspace.publish_batch(preview['batch_id'],'another-admin')
    result=workspace.publish_batch(preview['batch_id'],'admin')
    assert result['published_lessons']==1 and result['published_fields']==1
    assert result['failed'][0]['lesson_id']==context['lesson']['id'] and result['failed'][0]['code']=='draft_changed_reload'
    assert field_of(workspace,context)['status']=='draft' and field_of(workspace,other)['status']=='published'
    assert workspace.publish_batch(preview['batch_id'],'admin')==result
    assert workspace.context(other['lesson']['id'])['lesson']['title']==context['lesson']['title']


def test_bulk_publication_preview_expires(workspace,memory_context):
    context=memory_context
    save(workspace,context,field_of(workspace,context))
    preview=workspace.prepare_publication([context['lesson']['id']],'fa','admin')
    with workspace.connect() as c:
        c.execute("UPDATE localization_admin.publication_batches SET created_at=now()-interval '11 minutes' WHERE id=%s",(preview['batch_id'],))
    with pytest.raises(ValueError,match='publication_preview_expired'):
        workspace.publish_batch(preview['batch_id'],'admin')
    with pytest.raises(ValueError,match='batch_publication_scope_required'):
        workspace.prepare_publication([],'fa','admin')


def test_reused_phrase_publication_preserves_dictionary_origin_contract(workspace,memory_context,provider):
    context=memory_context
    save(workspace,context,field_of(workspace,context,'lesson_item'),'من انگلیسی صحبت می‌کنم.')
    other=clone_context(workspace,context);target=field_of(workspace,other,'lesson_item')
    result=workspace.enqueue([other['lesson']['id']],'fa',provider,5,[target['key']],False,'admin')
    assert result['reused']==1
    draft=field_of(workspace,other,'lesson_item')
    with workspace.connect() as c:
        c.execute("ALTER TABLE public.content_translations ADD CONSTRAINT test_public_origin CHECK(origin IN ('human','machine','source'))")
    workspace.publish(other['lesson']['id'],'fa',[draft['key']],{draft['key']:draft['revision']},'admin')
    assert field_of(workspace,other,'lesson_item')['status']=='published'
    with workspace.connect() as c:
        assert c.execute('SELECT origin FROM public.content_translations').fetchone()['origin']=='machine'


def test_bootstrap_indexes_existing_drafts_without_rekeying_old_rules(workspace,memory_context,monkeypatch):
    context=memory_context
    save(workspace,context,field_of(workspace,context))
    with workspace.connect() as c:
        c.execute('DELETE FROM localization_admin.memory_entries')
    assert workspace.index_memory()==1
    other=clone_context(workspace,context);target=field_of(workspace,other)
    assert workspace.preview_queue([other['lesson']['id']],'fa',[target['key']])['reusable']==1
    monkeypatch.setattr('localizer.admin_memory.versions',lambda:('new-rules','new-glossary'))
    assert workspace.index_memory()==0
    assert workspace.preview_queue([other['lesson']['id']],'fa',[target['key']])['fresh']==1


def test_cancel_waiting_releases_the_target_field(workspace,memory_context,provider):
    context=memory_context;other=clone_context(workspace,context)
    field=field_of(workspace,context);target=field_of(workspace,other)
    workspace.enqueue([context['lesson']['id']],'fa',provider,5,[field['key']],False,'admin')
    follower=workspace.enqueue([other['lesson']['id']],'fa',provider,5,[target['key']],False,'admin')
    workspace.control(follower['run_id'],'cancelled','admin')
    assert field_of(workspace,other)['status']=='missing'
    again=workspace.enqueue([other['lesson']['id']],'fa',provider,5,[target['key']],False,'admin')
    assert again['waiting']==1


def test_bulk_retry_recognizes_publication_committed_before_receipt(workspace,memory_context):
    context=memory_context;other=clone_context(workspace,context)
    for lesson in (context,other): save(workspace,lesson,field_of(workspace,lesson))
    preview=workspace.prepare_publication([context['lesson']['id'],other['lesson']['id']],'fa','admin')
    field=field_of(workspace,context)
    workspace.publish(context['lesson']['id'],'fa',[field['key']],{field['key']:field['revision']},'admin')
    result=workspace.publish_batch(preview['batch_id'],'admin')
    assert result=={'published_lessons':2,'published_fields':2,'failed':[]}
    with workspace.connect() as c:
        assert c.execute("SELECT count(*) n FROM localization_admin.audit WHERE action='publish'").fetchone()['n']==2
