import json
import os
from uuid import UUID, uuid4
import psycopg
from psycopg.rows import dict_row
from psycopg.types.json import Jsonb
from .content import plan, digest
from .resources import rules, versions
from .validation import Bundle, Review, validate


class Store:
    def __init__(self, dsn=None):
        self.dsn = dsn or os.environ['LOCALIZATION_DATABASE_URL']

    def connect(self):
        return psycopg.connect(self.dsn, row_factory=dict_row, options='-c statement_timeout=10000 -c lock_timeout=3000')

    def context(self, lesson_id, conn=None, lock=False):
        if conn is None:
            with self.connect() as c:
                c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY')
                return self.context(lesson_id, c)
        lesson_id = UUID(str(lesson_id))
        if lock:
            conn.execute('SELECT localization.lock_source()')
        suffix = ''
        lesson = conn.execute('SELECT * FROM public.lessons WHERE id=%s'+suffix, (lesson_id,)).fetchone()
        if not lesson:
            raise ValueError('lesson_not_found')
        course = conn.execute('SELECT * FROM public.courses WHERE id=%s'+suffix, (lesson['course_id'],)).fetchone()
        unit = conn.execute('SELECT * FROM public.units WHERE id=%s'+suffix, (lesson['unit_id'],)).fetchone() if lesson.get('unit_id') else None
        items = conn.execute('SELECT * FROM public.lesson_items WHERE lesson_id=%s ORDER BY position,id'+suffix, (lesson_id,)).fetchall()
        exercises = conn.execute('SELECT e.* FROM public.exercises e JOIN public.lesson_items i ON i.id=e.lesson_item_id WHERE i.lesson_id=%s ORDER BY e.id'+'', (lesson_id,)).fetchall()
        # Canonical JSON wire form (UUIDs/timestamps). No credentials or users.
        return json.loads(json.dumps(dict(lesson=lesson,course=course,unit=unit,items=items,exercises=exercises), default=str))

    def lessons(self, after=None, limit=20):
        if not 1 <= limit <= 50:
            raise ValueError('limit_out_of_range')
        with self.connect() as c:
            return c.execute("SELECT l.id,l.title,c.cefr_level FROM public.lessons l JOIN public.courses c ON c.id=l.course_id WHERE c.language='en' AND (%s::uuid IS NULL OR l.id>%s::uuid) ORDER BY l.id LIMIT %s", (after,after,limit)).fetchall()

    def enqueue(self, lesson_ids, model, review_model, budget, authorization_limit=None):
        if budget <= 0 or not 1 <= len(lesson_ids) <= 50:
            raise ValueError('budget_and_batch_required')
        run = uuid4()
        with self.connect() as c:
            # One shared authorization across all runs, including unknown/failed requests.
            # Serialize reservations across controller processes; never silently release funds.
            c.execute('SELECT pg_advisory_xact_lock(172901321)')
            if authorization_limit is not None:
                reserved=c.execute('SELECT COALESCE(sum(budget_usd),0) n FROM localization.runs').fetchone()['n']
                if float(reserved)+budget>authorization_limit:
                    raise ValueError('shared_authorized_budget_already_reserved')
            c.execute("INSERT INTO localization.runs(id,status,model,review_model,instruction_version,glossary_version,budget_usd) VALUES (%s,'running',%s,%s,%s,%s,%s)",(run,model,review_model,*versions(),budget))
            for lesson in lesson_ids:
                ctx = self.context(lesson,c,lock=True)
                h = plan(ctx)['context_hash']
                c.execute('UPDATE localization.drafts d SET needs_review=true FROM localization.jobs j WHERE d.job_id=j.id AND j.lesson_id=%s AND d.context_hash<>%s',(lesson,h))
                for locale in ('fa','ckb'):
                    c.execute('INSERT INTO localization.jobs(id,run_id,lesson_id,locale,source_hash) VALUES (%s,%s,%s,%s,%s) ON CONFLICT (lesson_id,locale,source_hash) DO NOTHING',(uuid4(),run,lesson,locale,h))
        return str(run)

    def claim(self, run_id, lease_seconds=120):
        token = uuid4()
        with self.connect() as c:
            # Serialize run controls and claims; SKIP LOCKED lets workers share the queue.
            run = c.execute('SELECT * FROM localization.runs WHERE id=%s FOR UPDATE',(UUID(run_id),)).fetchone()
            if not run or run['status'] != 'running':
                return None
            row = c.execute("SELECT * FROM localization.jobs WHERE run_id=%s AND (status='pending' OR (status='claimed' AND lease_until<now())) AND attempts<3 ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1",(run_id,)).fetchone()
            if not row:
                c.execute("UPDATE localization.jobs SET status='failed',error_code='retry_exhausted' WHERE run_id=%s AND status='claimed' AND lease_until<now() AND attempts>=3",(run_id,))
                remaining = c.execute("SELECT count(*) n FROM localization.jobs WHERE run_id=%s AND status IN ('pending','claimed')",(run_id,)).fetchone()['n']
                if not remaining:
                    c.execute("UPDATE localization.runs SET status=CASE WHEN EXISTS(SELECT 1 FROM localization.jobs WHERE run_id=%s AND status IN ('failed','stale')) THEN 'failed' ELSE 'completed' END, updated_at=now() WHERE id=%s",(run_id,run_id))
                return None
            c.execute("UPDATE localization.jobs SET status='claimed',claim_token=%s,lease_until=now()+%s*interval '1 second',attempts=attempts+1,updated_at=now() WHERE id=%s",(token,lease_seconds,row['id']))
            return {**row,'claim_token':token,'model':run['model'],'review_model':run['review_model'],'instruction_version':run['instruction_version'],'glossary_version':run['glossary_version']}

    def heartbeat(self, job_id, token):
        with self.connect() as c:
            row=c.execute("UPDATE localization.jobs SET lease_until=now()+interval '120 seconds' WHERE id=%s AND claim_token=%s AND status='claimed' AND lease_until>now() RETURNING id",(job_id,token)).fetchone()
            if not row:
                raise ValueError('claim_expired')

    def record_session(self, job_id, token, phase, session_id):
        column = 'session_id' if phase=='translate' else 'review_session_id'
        with self.connect() as c:
            row=c.execute(f"UPDATE localization.jobs SET {column}=%s WHERE id=%s AND claim_token=%s AND status='claimed' RETURNING id",(session_id,job_id,token)).fetchone()
            if not row:
                raise ValueError('claim_lost')

    def save(self, job_id, token, bundle: Bundle, review: Review):
        with self.connect() as c:
            job=c.execute('SELECT j.*,r.status run_status,r.model,r.review_model,r.instruction_version,r.glossary_version FROM localization.jobs j JOIN localization.runs r ON r.id=j.run_id WHERE j.id=%s FOR UPDATE OF r,j',(UUID(str(job_id)),)).fetchone()
            if job and str(job['claim_token'])==str(token) and job['status']=='draft':
                return {'status':'draft','saved':0,'already_saved':True}
            if not job or str(job['claim_token'])!=str(token) or job['status']!='claimed' or job['run_status']!='running':
                raise ValueError('invalid_or_paused_claim')
            if not c.execute('SELECT lease_until>now() alive FROM localization.jobs WHERE id=%s',(job_id,)).fetchone()['alive']:
                raise ValueError('claim_expired')
            # Parent/child FOR SHARE prevents concurrent updates/deletes during source check + save.
            mapping=plan(self.context(job['lesson_id'],c,lock=True))
            if mapping['context_hash'] != job['source_hash']:
                c.execute("UPDATE localization.jobs SET status='stale',error_code='source_changed',updated_at=now() WHERE id=%s",(job_id,))
                c.execute('UPDATE localization.drafts SET needs_review=true WHERE context_hash=%s',(job['source_hash'],))
                return {'status':'stale','saved':0}
            if (job['instruction_version'],job['glossary_version'])!=versions():
                raise ValueError('configuration_changed')
            issues=validate(bundle,review,mapping,job['locale'],rules()['glossary'])
            sources={f['key']:f for f in mapping['fields']}
            for t in bundle.translations:
                f=sources[t.key]
                c.execute("INSERT INTO localization.drafts(id,job_id,source_type,source_id,field_path,locale,source_hash,context_hash,source_text,translation,supplementary_note,issues,review,model,review_model,instruction_version,glossary_version) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s) ON CONFLICT DO NOTHING",(uuid4(),job_id,f['source_type'],f['source_id'],f['path'],bundle.locale,f['source_hash'],bundle.context_hash,f['text'],t.text,t.supplementary_note,Jsonb(issues[t.key]),Jsonb(review.model_dump()),job['model'],job['review_model'],job['instruction_version'],job['glossary_version']))
            c.execute("UPDATE localization.jobs SET status='draft',lease_until=NULL,updated_at=now() WHERE id=%s",(job_id,))
            return {'status':'draft','saved':len(bundle.translations)}

    def fail(self, job_id, token, code):
        # Only stable, sanitized codes; never store exceptions containing prompts/secrets.
        with self.connect() as c:
            c.execute("UPDATE localization.jobs SET status='failed',error_code=%s,updated_at=now() WHERE id=%s AND claim_token=%s AND status='claimed'",(code,job_id,token))

    def control(self, run_id, status):
        if status not in ('running','paused'):
            raise ValueError('invalid_control')
        with self.connect() as c:
            row=c.execute('UPDATE localization.runs SET status=%s,updated_at=now() WHERE id=%s RETURNING id',(status,UUID(run_id))).fetchone()
            if not row: raise ValueError('run_not_found')

    def status(self, run_id):
        with self.connect() as c:
            run=c.execute('SELECT id,status,model,review_model,budget_usd,created_at,updated_at FROM localization.runs WHERE id=%s',(UUID(run_id),)).fetchone()
            jobs=c.execute('SELECT id,lesson_id,locale,status,attempts,error_code,session_id,review_session_id FROM localization.jobs WHERE run_id=%s ORDER BY created_at,id',(run_id,)).fetchall()
            return {'run':run,'jobs':jobs}

    def export(self, run_id):
        with self.connect() as c:
            return c.execute('SELECT d.source_type,d.source_id,d.field_path,d.source_text,d.locale,d.translation,d.supplementary_note,d.issues,d.status,d.needs_review FROM localization.drafts d JOIN localization.jobs j ON j.id=d.job_id WHERE j.run_id=%s ORDER BY source_type,source_id,field_path,locale',(UUID(run_id),)).fetchall()

    def refresh(self, run_id):
        """Explicitly rescan live source versions; never modify translation text or approval."""
        changed=[]
        with self.connect() as c:
            ids=c.execute('SELECT DISTINCT lesson_id FROM localization.jobs WHERE run_id=%s',(UUID(run_id),)).fetchall()
            for row in ids:
                try:
                    current=plan(self.context(row['lesson_id'],c,lock=True))['context_hash']
                except ValueError:
                    current='deleted'
                result=c.execute('UPDATE localization.drafts d SET needs_review=true FROM localization.jobs j WHERE d.job_id=j.id AND j.lesson_id=%s AND d.context_hash<>%s AND NOT d.needs_review RETURNING d.id',(row['lesson_id'],current)).fetchall()
                changed.extend(str(r['id']) for r in result)
        return {'marked_needs_review':len(changed)}

    def needed(self, locale, after=None, limit=20):
        if locale not in ('fa','ckb'):
            raise ValueError('unsupported_locale')
        page=self.lessons(after,limit)
        needed=[]
        for row in page:
            context=self.context(row['id'])
            source_hash=plan(context)['context_hash']
            with self.connect() as c:
                existing=c.execute('SELECT status FROM localization.jobs WHERE lesson_id=%s AND locale=%s AND source_hash=%s',(row['id'],locale,source_hash)).fetchone()
            if not existing or existing['status']!='draft':
                needed.append({**row,'source_hash':source_hash,'status':existing['status'] if existing else 'needs_translation'})
        return {'lessons':needed,'next_cursor':str(page[-1]['id']) if len(page)==limit else None}
