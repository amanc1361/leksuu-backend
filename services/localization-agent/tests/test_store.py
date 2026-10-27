from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from uuid import uuid4
import psycopg
import pytest
from localizer.content import plan
from test_content import candidate,reviewed


def enqueue(store,context):
    return store.enqueue([context['lesson']['id']],'gpt-6.1-sol','gpt-6.1-sol',5)


def test_real_postgres_idempotence_and_atomic_drafts(store,context):
    run=enqueue(store,context); enqueue(store,context)
    with store.connect() as c: assert c.execute('SELECT count(*) n FROM localization.jobs').fetchone()['n']==2
    job=store.claim(run); mapping=plan(store.context(job['lesson_id']))
    bundle=candidate(mapping,job['locale']); review=reviewed(mapping)
    assert store.save(job['id'],job['claim_token'],bundle,review)['status']=='draft'
    with store.connect() as c:
        assert c.execute('SELECT count(*) n FROM localization.drafts').fetchone()['n']==len(mapping['fields'])
        assert c.execute("SELECT count(*) n FROM localization.drafts WHERE status='human_approved'").fetchone()['n']==0
    assert store.save(job['id'],job['claim_token'],bundle,review)['already_saved']


def test_claim_pause_lease_recovery_and_concurrency(store,context):
    run=enqueue(store,context)
    store.control(run,'paused'); assert store.claim(run) is None
    store.control(run,'running')
    with ThreadPoolExecutor(2) as pool: jobs=list(pool.map(lambda _:store.claim(run),range(2)))
    assert len({str(j['id']) for j in jobs})==2
    old=jobs[0]
    with store.connect() as c: c.execute("UPDATE localization.jobs SET lease_until=now()-interval '1 second' WHERE id=%s",(old['id'],))
    new=store.claim(run)
    assert new['id']==old['id'] and new['claim_token']!=old['claim_token']
    with pytest.raises(ValueError): store.heartbeat(old['id'],old['claim_token'])


def test_changed_answer_invalidates_save_and_never_changes_source(store,context):
    run=enqueue(store,context); job=store.claim(run); mapping=plan(store.context(job['lesson_id']))
    with store.connect() as c:
        c.execute("UPDATE public.exercises SET answer_key='{\"correct\":99}' WHERE id=%s",(context['exercises'][1]['id'],))
    assert store.save(job['id'],job['claim_token'],candidate(mapping,job['locale']),reviewed(mapping))['status']=='stale'
    with store.connect() as c: assert c.execute('SELECT count(*) n FROM localization.drafts').fetchone()['n']==0


def test_failed_validation_rolls_back_whole_bundle(store,context):
    run=enqueue(store,context); job=store.claim(run); mapping=plan(store.context(job['lesson_id']))
    bundle=candidate(mapping,job['locale']); bundle.translations[-1].text='wrong script'
    with pytest.raises(ValueError): store.save(job['id'],job['claim_token'],bundle,reviewed(mapping))
    with store.connect() as c: assert c.execute('SELECT count(*) n FROM localization.drafts').fetchone()['n']==0


def test_dedicated_role_cannot_modify_source_or_approve(store,context):
    with store.connect() as c:
        exists=c.execute("SELECT 1 FROM pg_roles WHERE rolname='localization_service'").fetchone()
        if exists: c.execute('DROP OWNED BY localization_service; DROP ROLE localization_service')
        c.execute((Path(__file__).parents[1]/'sql/002_role.sql').read_text())
    with store.connect() as c:
        c.execute('SET ROLE localization_service')
        c.execute('SELECT * FROM public.exercises LIMIT 1')
        c.execute('SELECT localization.lock_source()')
    for query in ["UPDATE public.exercises SET type='mcq'",'CREATE TABLE public.forbidden(id int)',"UPDATE localization.drafts SET status='human_approved'",'SELECT * FROM pg_authid']:
        with pytest.raises(psycopg.errors.InsufficientPrivilege):
            with store.connect() as c:
                c.execute('SET ROLE localization_service'); c.execute(query)


def test_migration_down_up(store):
    root=Path(__file__).parents[1]
    with store.connect() as c:
        c.execute((root/'sql/001_down.sql').read_text())
        c.execute((root/'sql/001_up.sql').read_text())
        assert c.execute("SELECT to_regclass('localization.drafts') found").fetchone()['found']


def test_human_approved_draft_not_overwritten_and_source_marks_review(store,context):
    run=enqueue(store,context); job=store.claim(run); mapping=plan(store.context(job['lesson_id']))
    store.save(job['id'],job['claim_token'],candidate(mapping,job['locale']),reviewed(mapping))
    with store.connect() as c:
        c.execute("UPDATE localization.drafts SET status='human_approved',needs_review=false")
        c.execute("UPDATE lessons SET title=title||' updated'")
    enqueue(store,context)
    with store.connect() as c:
        assert c.execute("SELECT count(*) n FROM localization.drafts WHERE status='human_approved' AND needs_review").fetchone()['n']==len(mapping['fields'])


def test_service_role_can_claim_and_save_without_source_update_privilege(store,context):
    from localizer.store import Store
    run=enqueue(store,context)
    # Use SET ROLE per connection to exercise the exact grants without a second password.
    with store.connect() as c:
        if not c.execute("SELECT 1 FROM pg_roles WHERE rolname='localization_service'").fetchone():
            c.execute((Path(__file__).parents[1]/'sql/002_role.sql').read_text())
        else:
            c.execute('GRANT USAGE ON SCHEMA public,localization TO localization_service')
            c.execute('GRANT SELECT ON ALL TABLES IN SCHEMA public TO localization_service')
            c.execute('GRANT SELECT,INSERT,UPDATE ON localization.runs,localization.jobs TO localization_service')
            c.execute('GRANT SELECT,INSERT ON localization.drafts TO localization_service')
            c.execute('GRANT UPDATE(needs_review) ON localization.drafts TO localization_service')
            c.execute('GRANT EXECUTE ON FUNCTION localization.lock_source() TO localization_service')
    class Restricted(Store):
        def connect(self):
            c=super().connect(); c.execute('SET ROLE localization_service'); c.commit(); return c
    restricted=Restricted(store.dsn)
    job=restricted.claim(run); mapping=plan(restricted.context(job['lesson_id']))
    assert restricted.save(job['id'],job['claim_token'],candidate(mapping,job['locale']),reviewed(mapping))['status']=='draft'


def test_refresh_detects_source_edit_without_reenqueue(store,context):
    run=enqueue(store,context); job=store.claim(run); mapping=plan(store.context(job['lesson_id']))
    store.save(job['id'],job['claim_token'],candidate(mapping,job['locale']),reviewed(mapping))
    with store.connect() as c:
        c.execute('UPDATE localization.drafts SET needs_review=false')
        c.execute("UPDATE lessons SET title=title||' changed'")
    assert store.refresh(run)['marked_needs_review']==len(mapping['fields'])


def test_shared_budget_reservation_prevents_two_full_pilots(store,context):
    store.enqueue([context['lesson']['id']],'gpt-6.1-sol','gpt-6.1-sol',3,authorization_limit=5)
    with pytest.raises(ValueError,match='shared_authorized_budget_already_reserved'):
        store.enqueue([context['lesson']['id']],'gpt-6-astra','gpt-6-astra',3,authorization_limit=5)
    with store.connect() as c:
        assert c.execute('SELECT count(*) n FROM localization.runs').fetchone()['n']==1


def test_cli_default_start_and_dry_run_do_not_write_database(store,context):
    import json,os,subprocess,sys
    root=Path(__file__).parents[1]
    env={**os.environ,'LOCALIZATION_DATABASE_URL':store.dsn}
    for command in ('dry-run','start'):
        result=subprocess.run([sys.executable,'-m','localizer.cli',command,'--lesson',context['lesson']['id']],cwd=root,env=env,capture_output=True,text=True)
        assert result.returncode==0,result.stdout
        assert json.loads(result.stdout)['writes']==0
    with store.connect() as c:
        assert c.execute('SELECT count(*) n FROM localization.runs').fetchone()['n']==0
        assert c.execute('SELECT count(*) n FROM localization.drafts').fetchone()['n']==0


def test_deployment_audit_handles_system_catalog_relations(store):
    import ast
    root=Path(__file__).parents[1]
    with store.connect() as c:
        if c.execute("SELECT 1 FROM pg_roles WHERE rolname='localization_service'").fetchone():
            c.execute('DROP OWNED BY localization_service; DROP ROLE localization_service')
        c.execute((root/'sql/002_role.sql').read_text())
        tree=ast.parse((root/'scripts/install-server.py').read_text())
        audit=next(ast.literal_eval(node.value) for node in ast.walk(tree) if isinstance(node,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='audit' for t in node.targets))
        c.execute(audit)
