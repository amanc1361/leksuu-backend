"""Persistent segment queue and explicit admin publication; never writes grading data."""
import copy
import json
from collections import defaultdict
from uuid import UUID, uuid4

from psycopg.types.json import Jsonb

from .admin_memory import MemoryMixin, memory_key
from .admin_batch import BatchPublicationMixin
from .content import plan as source_plan
from .publication import dictionary_hash
from .resources import rules
from .store import Store
from .validation import Bundle, Review, Translation, Finding, validate


def plan(context):
    mapping=source_plan(context)
    suffix='.'+context['course']['language']
    for field in mapping['fields']:
        # Turning a scalar into an en/fa/ckb map during publication must not
        # change the identity of the translatable source field.
        path=field['path'].removesuffix(suffix)
        field['key']=f"{field['source_type']}:{field['source_id']}:{path}"
    return mapping


def checked_text(field, locale, text):
    """Reuse all source-preservation checks for a single human or model draft."""
    mapping = {'context_hash': '0'*64, 'fields': [field]}
    bundle = Bundle(locale=locale, context_hash=mapping['context_hash'], translations=[
        Translation(key=field['key'], text=text, supplementary_note='', issues=[])])
    review = Review(findings=[Finding(key=field['key'], meaning='pass', completeness='pass',
        terminology='pass', language='pass', pedagogy='pass', issues=[])])
    return validate(bundle, review, mapping, locale, rules()['glossary'])[field['key']]


def location(context, field):
    """Return the actual localized leaf without ever targeting an answer/options field."""
    kind = field['source_type']
    language = context['course']['language']
    if kind == 'lesson':
        row = context['lesson']
        parts = ['objective', 'title'] if field['path'] == 'title' else field['path'].split('.')
    elif kind == 'exercise':
        row = next(e for e in context['exercises'] if str(e['id']) == field['source_id'])
        parts = field['path'].split('.')
    else:
        row = next(i for i in context['items'] if str(i['id']) == field['source_id'])
        parts = field['path'].split('.')
    if parts[-1] == language:
        parts.pop()
    for part in parts[:-1]:
        if isinstance(row, list):
            row = row[int(part)]
        else:
            if row.get(part) is None:
                row[part] = {}
            row = row[part]
    return row, parts[-1]


def inline(context, field, locale):
    row, leaf = location(copy.deepcopy(context), field)
    value = row[int(leaf)] if isinstance(row, list) else row.get(leaf)
    return value.get(locale, '') if isinstance(value, dict) else ''


class Workspace(MemoryMixin, BatchPublicationMixin, Store):
    def catalog(self, course_id=None, unit_id=None, locale='fa'):
        self.locale(locale)
        with self.connect() as c:
            c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY')
            courses = c.execute('SELECT id,title,language,cefr_level FROM public.courses ORDER BY language,cefr_level,title').fetchall()
            units, lessons = [], []
            if course_id:
                UUID(course_id)
                units = c.execute('SELECT id,title,position FROM public.units WHERE course_id=%s ORDER BY position,id', (course_id,)).fetchall()
                rows = c.execute('SELECT * FROM public.lessons WHERE course_id=%s AND (%s::uuid IS NULL OR unit_id=%s::uuid) ORDER BY position,id LIMIT 501', (course_id,unit_id,unit_id)).fetchall()
                ids=[r['id'] for r in rows[:500]]
                course=c.execute('SELECT * FROM public.courses WHERE id=%s',(course_id,)).fetchone()
                all_units={str(r['id']):r for r in c.execute('SELECT * FROM public.units WHERE course_id=%s',(course_id,)).fetchall()}
                items=defaultdict(list); exercises=defaultdict(list); records=defaultdict(list)
                if ids:
                    for item in c.execute('SELECT * FROM public.lesson_items WHERE lesson_id=ANY(%s) ORDER BY position,id',(ids,)).fetchall():
                        items[str(item['lesson_id'])].append(item)
                    for exercise in c.execute('SELECT e.*,i.lesson_id AS parent_lesson FROM public.exercises e JOIN public.lesson_items i ON i.id=e.lesson_item_id WHERE i.lesson_id=ANY(%s) ORDER BY e.id',(ids,)).fetchall():
                        parent=str(exercise.pop('parent_lesson')); exercises[parent].append(exercise)
                    for record in c.execute('SELECT * FROM localization_admin.segments WHERE lesson_id=ANY(%s) AND locale=%s',(ids,locale)).fetchall():
                        records[str(record['lesson_id'])].append(record)
                contexts=[]; hashes=[]
                for row in rows[:500]:
                    identity=str(row['id'])
                    context=json.loads(json.dumps(dict(lesson=row,course=course,unit=all_units.get(str(row['unit_id'])),items=items[identity],exercises=exercises[identity]),default=str))
                    contexts.append(context)
                    hashes.extend(dictionary_hash(f['text']) for f in plan(context)['fields'] if f['source_type']=='lesson_item')
                dictionary=c.execute('SELECT source_hash,text FROM public.content_translations WHERE locale=%s AND source_hash=ANY(%s)',(locale,hashes)).fetchall() if hashes else []
                translated={r['source_hash']:r['text'] for r in dictionary}
                for row,context in zip(rows[:500],contexts):
                    # Counts are computed from actual sources, inline translations and drafts.
                    detail = self.present(context,locale,records[str(row['id'])],translated)
                    counts = {name: sum(f['status'] == name for f in detail['fields']) for name in ('missing','draft','published','failed','unknown','pending','running')}
                    lessons.append({k:row[k] for k in ('id','title','unit_id','position')} | {'counts':counts, 'total':len(detail['fields']), 'unmapped':len(detail['ambiguous'])})
            return dict(courses=courses, units=units, lessons=lessons,
                        truncated=bool(course_id and len(rows)>500), locales=['fa','ckb'])

    @staticmethod
    def locale(locale):
        if locale not in ('fa','ckb'):
            raise ValueError('unsupported_locale')

    def lesson(self, lesson_id, locale, conn=None):
        self.locale(locale)
        if conn is None:
            with self.connect() as c:
                c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY')
                return self.lesson(lesson_id,locale,c)
        context = self.context(lesson_id, conn)
        mapping = plan(context)
        records = conn.execute('SELECT * FROM localization_admin.segments WHERE lesson_id=%s AND locale=%s', (lesson_id,locale)).fetchall()
        indexed = {(r['field_key'],r['source_hash']):r for r in records}
        hashes = [dictionary_hash(f['text']) for f in mapping['fields'] if f['source_type']=='lesson_item']
        dictionary = conn.execute('SELECT source_hash,text FROM public.content_translations WHERE locale=%s AND source_hash=ANY(%s)', (locale,hashes)).fetchall() if hashes else []
        translated = {r['source_hash']:r['text'] for r in dictionary}
        return self.present(context,locale,records,translated)

    def present(self,context,locale,records,translated):
        mapping=plan(context)
        indexed={(r['field_key'],r['source_hash']):r for r in records}
        fields = []
        for f in mapping['fields']:
            record = indexed.get((f['key'],f['source_hash']))
            published = inline(context,f,locale)
            if not published and f['source_type']=='lesson_item':
                published = translated.get(dictionary_hash(f['text']), '')
            # The original pilot retained pre-publication drafts even after editorial
            # corrections were published. Show the actual live translation for those
            # untouched imports; a subsequent manual edit gets its own new revision.
            untouched_legacy=record and record.get('legacy_imported') and record['revision']==1
            text = published if untouched_legacy and published else (record['translation'] if record and record['translation'] else published)
            status = record['status'] if record else 'missing'
            if published and text == published and status not in ('pending','running','failed','unknown'):
                status = 'published'
            fields.append({**f,'id':str(record['id']) if record else None,'translation':text,
                'published_text':published,'status':status,'issues':record['issues'] if record else [],
                'error_code':record['error_code'] if record else None,
                'revision':record['revision'] if record else 0,
                'model':record['model'] if record else None,
                'origin':record['origin'] if record else None,
                'memory_source_id':str(record['memory_source_id']) if record and record.get('memory_source_id') else None,
                'stale_previous':any(r['field_key']==f['key'] and r['source_hash']!=f['source_hash'] for r in records)})
        return dict(lesson_id=str(context['lesson']['id']),title=context['lesson']['title'],source_language=context['course']['language'],
                    locale=locale,context_hash=mapping['context_hash'],fields=fields,ambiguous=mapping['ambiguous'])

    def ensure(self,c,lesson_id,locale,field):
        return c.execute('INSERT INTO localization_admin.segments(id,lesson_id,field_key,source_hash,source_text,locale) VALUES (%s,%s,%s,%s,%s,%s) ON CONFLICT(lesson_id,field_key,source_hash,locale) DO UPDATE SET source_text=excluded.source_text RETURNING *',
            (uuid4(),lesson_id,field['key'],field['source_hash'],field['text'],locale)).fetchone()

    def audit(self,c,actor,action,segment_id,detail):
        wire=json.loads(json.dumps(detail,default=str,ensure_ascii=False))
        c.execute('INSERT INTO localization_admin.audit(actor,action,segment_id,detail) VALUES (%s,%s,%s,%s)', (actor,action,segment_id,Jsonb(wire)))

    def manual(self,lesson_id,locale,key,source_hash,text,revision,actor):
        self.locale(locale)
        if not isinstance(text,str) or not text.strip():
            raise ValueError('translation_required')
        with self.connect() as c:
            c.execute('SELECT localization.lock_source()')
            field = self.field(c,lesson_id,key,source_hash)
            issues = checked_text(field,locale,text)
            record = self.ensure(c,lesson_id,locale,field)
            if record['revision'] != revision:
                raise ValueError('draft_changed_reload')
            c.execute("UPDATE localization_admin.segments SET translation=%s,origin='human',memory_source_id=NULL,status='draft',issues=%s,error_code=NULL,revision=revision+1,updated_at=now() WHERE id=%s", (text,Jsonb(issues),record['id']))
            self.remember(c,self.context(lesson_id,c),field,locale,record['id'])
            self.audit(c,actor,'manual_draft',record['id'],{'before':record['translation'],'after':text})
        return self.lesson(lesson_id,locale)

    def field(self,c,lesson_id,key,source_hash):
        fields = plan(self.context(lesson_id,c))['fields']
        field = next((f for f in fields if f['key']==key and f['source_hash']==source_hash),None)
        if not field:
            raise ValueError('source_changed_reload')
        return field

    def enqueue(self,lesson_ids,locale,provider,budget,keys,force,actor):
        self.locale(locale)
        if not 1 <= len(lesson_ids) <= 500 or not 0 < budget <= 50:
            raise ValueError('scope_and_budget_required')
        lesson_ids=list(dict.fromkeys(str(UUID(identity)) for identity in lesson_ids))
        if keys and len(lesson_ids)!=1:
            raise ValueError('field_selection_requires_one_lesson')
        run_id=uuid4()
        queued=skipped=reused=waiting=fresh=review=0
        cache={}
        with self.connect() as c:
            c.execute('SELECT localization.lock_source()')
            c.execute('SELECT pg_advisory_xact_lock(172901324)')
            snapshot={k:v for k,v in provider.items() if k!='api_key'}
            scope=self.run_scope(c,lesson_ids)
            if len(scope)!=len(lesson_ids):
                raise ValueError('lesson_not_found')
            c.execute('INSERT INTO localization_admin.runs(id,provider_id,model,locale,budget_usd,provider_snapshot,scope) VALUES (%s,%s,%s,%s,%s,%s,%s)', (run_id,provider['id'],provider['model'],locale,budget,Jsonb(snapshot),Jsonb(scope)))
            for lesson_id in lesson_ids:
                context=self.context(lesson_id,c)
                detail=self.lesson(lesson_id,locale,c)
                if keys and not set(keys)<={f['key'] for f in detail['fields']}:
                    raise ValueError('unknown_field')
                for field in detail['fields']:
                    if keys and field['key'] not in keys:
                        continue
                    if field['status'] in ('pending','running') or (not force and (field['translation'] or field['status']=='unknown')):
                        skipped+=1
                        continue
                    record=self.ensure(c,lesson_id,locale,field)
                    active=c.execute("SELECT id FROM localization_admin.tasks WHERE segment_id=%s AND status IN ('pending','running','waiting')",(record['id'],)).fetchone()
                    if active:
                        skipped+=1
                        continue
                    signature,state,source=(None,'fresh',None) if force else self.memory_lookup(c,context,field,locale,cache)
                    parent=c.execute("SELECT id FROM localization_admin.tasks WHERE memory_key=%s AND status IN ('pending','running') ORDER BY created_at,id LIMIT 1",(signature,)).fetchone() if signature else None
                    status='pending'
                    if state=='reuse':
                        self.reuse(c,context,field,locale,record,source,actor)
                        status='reused';reused+=1
                    elif state=='review':
                        status='failed';review+=1
                        c.execute("UPDATE localization_admin.segments SET status='failed',error_code='translation_memory_requires_review' WHERE id=%s",(record['id'],))
                    elif parent:
                        status='waiting';waiting+=1;queued+=1
                        c.execute("UPDATE localization_admin.segments SET status='pending',error_code=NULL WHERE id=%s",(record['id'],))
                    else:
                        fresh+=1;queued+=1
                        c.execute("UPDATE localization_admin.segments SET status='pending',error_code=NULL WHERE id=%s",(record['id'],))
                    c.execute('INSERT INTO localization_admin.tasks(id,run_id,segment_id,expected_revision,status,memory_key,depends_on,error_code) VALUES (%s,%s,%s,%s,%s,%s,%s,%s)',(uuid4(),run_id,record['id'],record['revision'],status,signature,parent['id'] if status=='waiting' else None,'translation_memory_requires_review' if state=='review' else None))
            if not queued:
                c.execute('UPDATE localization_admin.runs SET status=%s WHERE id=%s',('failed' if review else 'completed' if reused else 'skipped',run_id))
            self.audit(c,actor,'enqueue',None,{'run_id':str(run_id),'queued':queued,'skipped':skipped,'reused':reused,'waiting':waiting,'fresh':fresh,'review':review,'budget_usd':budget,'force':force})
        return dict(run_id=str(run_id),queued=queued,skipped=skipped,reused=reused,waiting=waiting,fresh=fresh,review=review)

    @staticmethod
    def run_scope(c,lesson_ids):
        rows=c.execute("""SELECT l.id lesson_id,l.title lesson_title,l.position lesson_position,
            u.id unit_id,u.title unit_title,u.position unit_position,
            c.id course_id,c.title course_title,c.language source_language,c.cefr_level
            FROM public.lessons l JOIN public.courses c ON c.id=l.course_id
            LEFT JOIN public.units u ON u.id=l.unit_id
            WHERE l.id=ANY(%s::uuid[]) ORDER BY c.title,u.position,l.position,l.id""",(list(lesson_ids),)).fetchall()
        return json.loads(json.dumps(rows,default=str))

    def runs(self):
        with self.connect() as c:
            c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY')
            runs=c.execute("""SELECT r.id,r.model,r.locale,r.status,r.budget_usd,r.reserved_usd,
                r.estimated_usd,r.created_at,r.scope,
                COALESCE(jsonb_object_agg(t.status,t.n) FILTER(WHERE t.status IS NOT NULL),'{}') counts
                FROM localization_admin.runs r LEFT JOIN
                (SELECT run_id,status,count(*) n FROM localization_admin.tasks GROUP BY run_id,status) t
                ON t.run_id=r.id GROUP BY r.id ORDER BY r.created_at DESC LIMIT 30""").fetchall()
            if not runs:
                return []
            rows=c.execute("""SELECT t.run_id,s.lesson_id,count(*) fields,
                count(*) FILTER(WHERE s.status='draft' AND s.translation<>'') drafts,
                count(*) FILTER(WHERE s.status='published' AND s.translation<>'') published,
                count(*) FILTER(WHERE s.issues<>'[]'::jsonb) review_fields,
                COALESCE(jsonb_agg(DISTINCT t.error_code) FILTER(WHERE t.error_code IS NOT NULL),'[]') errors
                FROM localization_admin.tasks t JOIN localization_admin.segments s ON s.id=t.segment_id
                WHERE t.run_id=ANY(%s) GROUP BY t.run_id,s.lesson_id""",([r['id'] for r in runs],)).fetchall()
            stats={(str(row['run_id']),str(row['lesson_id'])):row for row in rows}
            # Old runs predate stored scope; reconstruct their original lesson names.
            fallback={row['lesson_id']:row for row in self.run_scope(c,list({str(row['lesson_id']) for row in rows}))}
            for run in runs:
                identity=str(run['id'])
                scope=run.pop('scope') or [fallback[str(row['lesson_id'])] for row in rows if str(row['run_id'])==identity]
                run['lessons']=[]
                for lesson in scope:
                    counts=stats.get((identity,str(lesson['lesson_id'])),{})
                    run['lessons'].append({**lesson,**{k:counts.get(k,[] if k=='errors' else 0) for k in ('fields','drafts','published','review_fields','errors')}})
                run['drafts']=sum(l['drafts'] for l in run['lessons'])
                run['published']=sum(l['published'] for l in run['lessons'])
            return runs

    def control(self,run_id,status,actor):
        if status not in ('running','paused','cancelled'):
            raise ValueError('invalid_control')
        with self.connect() as c:
            c.execute('UPDATE localization_admin.runs SET status=%s WHERE id=%s AND status IN (\'running\',\'paused\')', (status,UUID(run_id)))
            if status=='cancelled':
                rows=c.execute("UPDATE localization_admin.tasks SET status='cancelled',updated_at=now() WHERE run_id=%s AND status IN ('pending','waiting') RETURNING segment_id",(run_id,)).fetchall()
                for row in rows:
                    c.execute("UPDATE localization_admin.segments SET status=CASE WHEN translation<>'' THEN 'draft' ELSE 'missing' END,error_code=NULL WHERE id=%s AND status='pending'",(row['segment_id'],))
            self.audit(c,actor,status,None,{'run_id':run_id})

    def publish(self,lesson_id,locale,keys,revisions,actor,replace_shared=False,expected_context_hash=None):
        self.locale(locale)
        if not keys or len(keys)>300:
            raise ValueError('explicit_field_selection_required')
        with self.connect() as c:
            c.execute('SELECT pg_advisory_xact_lock(172901322)')
            c.execute('SELECT localization.lock_source()')
            before=self.context(lesson_id,c)
            mapping=plan(before)
            if expected_context_hash and mapping['context_hash']!=expected_context_hash:
                raise ValueError('source_changed_reload')
            fields={f['key']:f for f in mapping['fields']}
            if not set(keys)<=fields.keys():
                raise ValueError('source_changed_reload')
            after=copy.deepcopy(before)
            for key in dict.fromkeys(keys):
                field=fields[key]
                record=c.execute('SELECT * FROM localization_admin.segments WHERE lesson_id=%s AND field_key=%s AND source_hash=%s AND locale=%s FOR UPDATE',(lesson_id,key,field['source_hash'],locale)).fetchone()
                if not record or not record['translation'] or record['status'] not in ('draft','published'):
                    raise ValueError('reviewed_draft_required')
                if revisions.get(key)!=record['revision']:
                    raise ValueError('draft_changed_reload')
                checked_text(field,locale,record['translation'])
                old=inline(before,field,locale)
                if field['source_type']=='lesson_item':
                    h=dictionary_hash(field['text'])
                    prior=c.execute('SELECT * FROM public.content_translations WHERE source_hash=%s AND locale=%s FOR UPDATE',(h,locale)).fetchone()
                    # Shared dictionary affects other lessons. Require a separate explicit
                    # review when replacing it; the UI surfaces this conflict.
                    if prior and prior['text']!=record['translation'] and not replace_shared:
                        raise ValueError('shared_translation_conflict')
                    if old and old!=record['translation']:
                        row,leaf=location(after,field)
                        value=dict(row[leaf])
                        value[locale]=record['translation']
                        row[leaf]=value
                    saved=c.execute('INSERT INTO public.content_translations(source_hash,locale,source_lang,source_text,text,origin) VALUES (%s,%s,%s,%s,%s,%s) ON CONFLICT(source_hash,locale) DO UPDATE SET text=excluded.text,origin=excluded.origin WHERE content_translations.text=excluded.text OR (%s AND content_translations.text=%s) RETURNING source_hash',(h,locale,before['course']['language'],field['text'],record['translation'],'human' if record['origin']=='human' else 'machine',replace_shared,prior['text'] if prior else None)).fetchone()
                    if not saved:
                        raise ValueError('shared_translation_conflict')
                    previous=prior
                else:
                    row,leaf=location(after,field)
                    value=row[int(leaf)] if isinstance(row,list) else row.get(leaf)
                    value=dict(value) if isinstance(value,dict) else {before['course']['language']:field['text']}
                    value.setdefault(before['course']['language'],field['text'])
                    previous=copy.deepcopy(value)
                    value[locale]=record['translation']
                    if isinstance(row,list): row[int(leaf)]=value
                    else: row[leaf]=value
                self.remember(c,before,field,locale,record['id'])
                self.audit(c,actor,'publish',record['id'],{'before':previous,'after':record['translation'],'source_hash':field['source_hash'],'native_review_verified':False})
                c.execute("UPDATE localization_admin.segments SET status='published',updated_at=now() WHERE id=%s",(record['id'],))
            # Write ONLY whitelisted localized leaves. Answers/options/media stay intact.
            c.execute('UPDATE public.lessons SET objective=%s WHERE id=%s',(Jsonb(after['lesson'].get('objective') or {}),UUID(lesson_id)))
            for exercise in after['exercises']:
                if exercise['prompt']!=next(e['prompt'] for e in before['exercises'] if e['id']==exercise['id']):
                    c.execute('UPDATE public.exercises SET prompt=%s WHERE id=%s',(Jsonb(exercise['prompt']),UUID(exercise['id'])))
            for item in after['items']:
                if item['payload']!=next(i['payload'] for i in before['items'] if i['id']==item['id']):
                    c.execute('UPDATE public.lesson_items SET payload=%s WHERE id=%s',(Jsonb(item['payload']),UUID(item['id'])))
            if self.context(lesson_id,c)!=after:
                raise ValueError('unexpected_source_mutation')
        return self.lesson(lesson_id,locale)
