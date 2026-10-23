"""Single durable admin worker. Unknown inference outcomes are never auto-retried."""
import json
import threading

from psycopg.types.json import Jsonb

from .admin_providers import call
from .admin_store import plan
from .hosted import HostedError
from .resources import rules
from .validation import Bundle, Review, validate

MAX_OUTPUT=4096


class Worker:
    def __init__(self,store,providers):
        self.store,self.providers=store,providers
        self.stop=threading.Event()

    def recover(self):
        # A dead HTTP request can still be billed. Preserve reservation and require
        # an explicit admin retry. Only a single workspace instance is supported.
        with self.store.connect() as c:
            rows=c.execute("UPDATE localization_admin.tasks SET status='unknown',error_code='worker_interrupted_outcome_unknown',updated_at=now() WHERE status='running' RETURNING segment_id,run_id").fetchall()
            for row in rows:
                c.execute("UPDATE localization_admin.segments SET status='unknown',error_code='worker_interrupted_outcome_unknown' WHERE id=%s AND status='running'",(row['segment_id'],))
                c.execute("UPDATE localization_admin.runs SET status='paused' WHERE id=%s AND status='running'",(row['run_id'],))

    def claim(self):
        with self.store.connect() as c:
            task=c.execute("SELECT t.*,s.lesson_id,s.field_key,s.source_hash,s.locale,r.provider_id,r.provider_snapshot FROM localization_admin.tasks t JOIN localization_admin.runs r ON r.id=t.run_id JOIN localization_admin.segments s ON s.id=t.segment_id WHERE t.status='pending' AND r.status='running' ORDER BY t.created_at,t.id FOR UPDATE OF t SKIP LOCKED LIMIT 1").fetchone()
            if task:
                c.execute("UPDATE localization_admin.tasks SET status='running',updated_at=now() WHERE id=%s",(task['id'],))
                c.execute("UPDATE localization_admin.segments SET status='running' WHERE id=%s AND revision=%s",(task['segment_id'],task['expected_revision']))
            else:
                c.execute("UPDATE localization_admin.runs r SET status=CASE WHEN EXISTS(SELECT 1 FROM localization_admin.tasks t WHERE t.run_id=r.id AND t.status IN ('failed','unknown')) THEN 'failed' WHEN EXISTS(SELECT 1 FROM localization_admin.tasks t WHERE t.run_id=r.id AND t.status='completed') THEN 'completed' ELSE 'skipped' END WHERE r.status='running' AND NOT EXISTS(SELECT 1 FROM localization_admin.tasks t WHERE t.run_id=r.id AND t.status IN ('pending','running'))")
            return task

    def reserve(self,task,provider,prompt,schema):
        # Conservative UTF-8 byte proxy includes repeated context/schema and reasoning.
        # Rates are configured by the admin and must cover the chosen context tier.
        size=len(json.dumps(prompt,ensure_ascii=False,default=str).encode())+len(json.dumps(schema).encode())+8192
        if size>190000:
            raise HostedError('input_too_large_split_required')
        cost=(size*provider['input_rate']+MAX_OUTPUT*provider['output_rate'])/1e6
        with self.store.connect() as c:
            row=c.execute('SELECT * FROM localization_admin.runs WHERE id=%s FOR UPDATE',(task['run_id'],)).fetchone()
            if row['status']!='running':
                raise HostedError('run_paused_before_submission')
            if float(row['reserved_usd'])+cost>float(row['budget_usd']):
                raise HostedError('run_budget_exhausted')
            c.execute('UPDATE localization_admin.runs SET reserved_usd=reserved_usd+%s WHERE id=%s',(cost,task['run_id']))

    def phase(self,task,provider,instruction,prompt,schema,phase):
        with self.store.connect() as c:
            revision=c.execute('SELECT revision FROM localization_admin.segments WHERE id=%s',(task['segment_id'],)).fetchone()
            if not revision or revision['revision']!=task['expected_revision']:
                raise HostedError('manual_edit_preserved')
        self.reserve(task,provider,{'instruction':instruction,'task':prompt},schema)
        # Persist phase before network I/O; on crash it becomes unknown, never replayed.
        with self.store.connect() as c:
            c.execute('UPDATE localization_admin.tasks SET phase=%s,updated_at=now() WHERE id=%s',(phase,task['id']))
        output,usage=call(provider,instruction,prompt,schema,MAX_OUTPUT)
        usd=(usage['input_tokens']*provider['input_rate']+usage['output_tokens']*provider['output_rate'])/1e6
        with self.store.connect() as c:
            c.execute('UPDATE localization_admin.tasks SET usage=usage||%s::jsonb WHERE id=%s',(Jsonb([{'phase':phase,**usage,'estimated_usd':usd}]),task['id']))
            c.execute('UPDATE localization_admin.runs SET estimated_usd=estimated_usd+%s WHERE id=%s',(usd,task['run_id']))
        return output

    def process(self,task):
        try:
            provider=self.providers.get(task['provider_id']) | task['provider_snapshot']
            context=self.store.context(task['lesson_id'])
            mapping=plan(context)
            field=next((f for f in mapping['fields'] if f['key']==task['field_key'] and f['source_hash']==task['source_hash']),None)
            if not field:
                raise HostedError('source_changed_reload')
            single={**mapping,'fields':[field]}
            instruction=rules()['instructions']
            if context['course']['language']!='en':
                instruction=instruction.replace('English',context['course']['language'])
            instruction+='\nThe controller supplied get_rules and get_lesson_context below. No tools are available. Translate ONLY the one supplied field. Return the supplied locale and context_hash exactly.'
            prompt={'locale':task['locale'],'context':context,**single,'rules':rules()}
            bundle=Bundle.model_validate(self.phase(task,provider,instruction,prompt,Bundle.model_json_schema(),'translate'))
            with self.store.connect() as c:
                c.execute('UPDATE localization_admin.tasks SET candidate=%s WHERE id=%s',(Jsonb(bundle.model_dump()),task['id']))
            review_prompt={**prompt,'candidate':bundle.model_dump()}
            review_instruction=instruction+'\nYou are an independent reviewer. Read the source yourself. Assess every field for meaning, completeness, terminology, language and pedagogy. Return findings only.'
            review=Review.model_validate(self.phase(task,provider,review_instruction,review_prompt,Review.model_json_schema(),'review'))
            issues=validate(bundle,review,single,task['locale'],rules()['glossary'])
            with self.store.connect() as c:
                c.execute('SELECT localization.lock_source()')
                current=self.store.context(task['lesson_id'],c)
                if plan(current)['context_hash']!=mapping['context_hash']:
                    raise HostedError('source_changed_during_generation')
                record=c.execute('SELECT * FROM localization_admin.segments WHERE id=%s FOR UPDATE',(task['segment_id'],)).fetchone()
                if record['revision']!=task['expected_revision']:
                    c.execute("UPDATE localization_admin.tasks SET status='skipped',error_code='manual_edit_preserved' WHERE id=%s",(task['id'],))
                    return
                candidate=bundle.translations[0]
                c.execute("UPDATE localization_admin.segments SET translation=%s,origin='machine',status='draft',issues=%s,provider_id=%s,model=%s,error_code=NULL,revision=revision+1,updated_at=now() WHERE id=%s",(candidate.text,Jsonb(issues[field['key']]),provider['id'],provider['model'],record['id']))
                c.execute("UPDATE localization_admin.tasks SET status='completed',review=%s,updated_at=now() WHERE id=%s",(Jsonb(review.model_dump()),task['id']))
                self.store.audit(c,'worker','ai_draft',record['id'],{'model':provider['model'],'task_id':str(task['id']),'before':record['translation'],'after':candidate.text})
        except Exception as error:
            code=str(error) if isinstance(error,(ValueError,HostedError)) else type(error).__name__
            if len(code)>100 or not code.replace('_','').isalnum():
                code='validation_failed'
            status='unknown' if code=='provider_outcome_unknown' else ('skipped' if code=='manual_edit_preserved' else 'failed')
            with self.store.connect() as c:
                c.execute('UPDATE localization_admin.tasks SET status=%s,error_code=%s,updated_at=now() WHERE id=%s',(status,code,task['id']))
                c.execute('UPDATE localization_admin.segments SET status=%s,error_code=%s,updated_at=now() WHERE id=%s AND revision=%s',(status,code,task['segment_id'],task['expected_revision']))
                if code in ('run_budget_exhausted','provider_http_401','provider_http_403','provider_http_429','provider_outcome_unknown'):
                    c.execute("UPDATE localization_admin.runs SET status='paused' WHERE id=%s",(task['run_id'],))

    def loop(self):
        self.recover()
        while not self.stop.is_set():
            try:
                task=self.claim()
                if task:
                    self.process(task)
                    continue
            except Exception:
                # No source, keys, DB DSNs or raw exception bodies in logs.
                pass
            self.stop.wait(2)
