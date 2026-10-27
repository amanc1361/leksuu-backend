"""Exact, contextual translation memory; reuse never submits a provider request."""
import re
from uuid import UUID, uuid4

from psycopg.types.json import Jsonb

from .content import digest
from .resources import versions


BASELINE_NOTES = {'native_sorani_review_required'}


def source_value(value, language):
    if isinstance(value, dict):
        if language in value:
            return source_value(value[language], language)
        return {k: source_value(v, language) for k, v in value.items()
                if k not in ('fa', 'ckb', 'audio', 'video', 'image', 'media', 'id')}
    if isinstance(value, list):
        return [source_value(v, language) for v in value]
    return value


def memory_key(context, field, locale):
    language = context['course']['language']
    path = field['path'].removesuffix('.' + language)
    purpose = field['source_type'] + ':' + re.sub(r'\.\d+(?=\.|$)', '.*', path)
    objective = context['lesson'].get('objective') or {}
    topic = [context['lesson']['title'], source_value(objective.get('summary'), language)]
    if field['source_type'] == 'lesson':
        anchors = topic
    elif field['source_type'] == 'lesson_item':
        item = next(i for i in context['items'] if str(i['id']) == field['source_id'])
        p = item.get('payload') or {}
        if item['kind'] == 'grammar':
            anchors = [source_value(p.get(k), language) for k in ('title', 'rule')]
        else:
            anchors = [p.get('example'), source_value(p.get('note'), language)]
            # Short words can have different meanings in different teaching topics.
            if len(field['text'].split()) < 3:
                anchors.append(topic)
    else:
        exercise = next(e for e in context['exercises'] if str(e['id']) == field['source_id'])
        if path in ('prompt.instruction', 'prompt.question'):
            anchors = exercise['type']
        else:
            anchors = [exercise['type'], source_value({k: v for k, v in exercise['prompt'].items()
                        if k not in ('instruction', 'explanation')}, language)]
    return digest({'version': 1, 'source_language': language, 'locale': locale,
                   'purpose': purpose, 'source': field['text'], 'anchors': anchors, 'rules': versions()})


class MemoryMixin:
    def remember(self, c, context, field, locale, segment_id, bootstrap=False):
        signature = memory_key(context, field, locale)
        conflict = 'DO NOTHING' if bootstrap else 'DO UPDATE SET signature=excluded.signature,updated_at=now()'
        c.execute('INSERT INTO localization_admin.memory_entries(segment_id,signature) VALUES (%s,%s) '
                  'ON CONFLICT(segment_id) ' + conflict, (segment_id, signature))

    def index_memory(self):
        from .admin_store import plan
        indexed = 0
        with self.connect() as c:
            c.execute('SELECT localization.lock_source()')
            rows = c.execute("SELECT s.* FROM localization_admin.segments s LEFT JOIN localization_admin.memory_entries m ON m.segment_id=s.id WHERE m.segment_id IS NULL AND s.translation<>'' AND s.status IN ('draft','published')").fetchall()
            contexts = {}
            for row in rows:
                identity = str(row['lesson_id'])
                if identity not in contexts:
                    contexts[identity] = self.context(identity, c)
                context = contexts[identity]
                field = next((f for f in plan(context)['fields'] if f['key'] == row['field_key'] and f['source_hash'] == row['source_hash']), None)
                if field:
                    self.remember(c, context, field, row['locale'], row['id'], bootstrap=True)
                    indexed += 1
        return indexed

    def memory_lookup(self, c, context, field, locale, cache=None):
        from .admin_store import plan, checked_text
        signature = memory_key(context, field, locale)
        rows = c.execute("SELECT s.* FROM localization_admin.memory_entries m JOIN localization_admin.segments s ON s.id=m.segment_id WHERE m.signature=%s AND s.translation<>'' AND s.status IN ('draft','published') ORDER BY (s.status='published') DESC,s.updated_at DESC,s.id", (signature,)).fetchall()
        candidates, flagged = [], False
        contexts = cache if cache is not None else {}
        for row in rows:
            identity = str(row['lesson_id'])
            if identity not in contexts:
                contexts[identity] = self.context(identity, c)
            current = contexts[identity]
            live = next((f for f in plan(current)['fields'] if f['key'] == row['field_key'] and f['source_hash'] == row['source_hash']), None)
            if not live or memory_key(current, live, locale) != signature:
                continue
            # The original pilot may have an editorial correction published over its draft.
            if row.get('legacy_imported') and row['revision'] == 1:
                detail = self.lesson(identity, locale, c)
                actual = next(f for f in detail['fields'] if f['key'] == live['key'])
                row = {**row, 'translation': actual['translation']}
            if set(row['issues']) - BASELINE_NOTES:
                flagged = True
                continue
            try:
                notes = checked_text(field, locale, row['translation'])
            except ValueError:
                flagged = True
                continue
            if set(notes) - BASELINE_NOTES:
                flagged = True
                continue
            candidates.append(row)
        if len({r['translation'] for r in candidates}) > 1:
            return signature, 'review', None
        if candidates:
            return signature, 'reuse', candidates[0]
        return signature, 'review' if flagged else 'fresh', None

    def reuse(self, c, context, field, locale, record, source, actor):
        from .admin_store import checked_text
        notes = sorted(set(source['issues']) | set(checked_text(field, locale, source['translation'])))
        updated = c.execute("UPDATE localization_admin.segments SET translation=%s,origin='memory',status='draft',issues=%s,model=%s,memory_source_id=%s,error_code=NULL,revision=revision+1,legacy_imported=false,updated_at=now() WHERE id=%s AND revision=%s RETURNING id", (source['translation'], Jsonb(notes), source['model'], source['id'], record['id'], record['revision'])).fetchone()
        if not updated:
            raise ValueError('draft_changed_reload')
        self.remember(c, context, field, locale, record['id'])
        self.audit(c, actor, 'memory_reuse', record['id'], {'source_segment_id': str(source['id']), 'provider_requests': 0})

    def preview_queue(self, lesson_ids, locale, keys=None, force=False):
        self.locale(locale)
        if not isinstance(lesson_ids, list) or not 1 <= len(lesson_ids) <= 500:
            raise ValueError('scope_and_budget_required')
        identities = list(dict.fromkeys(str(UUID(i)) for i in lesson_ids))
        if keys and len(identities) != 1:
            raise ValueError('field_selection_requires_one_lesson')
        result = dict(total=0, existing=0, active=0, reusable=0, waiting=0, fresh=0, review=0)
        seen, cache = set(), {}
        with self.connect() as c:
            c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY')
            for identity in identities:
                context = self.context(identity, c)
                detail = self.lesson(identity, locale, c)
                if keys and not set(keys) <= {f['key'] for f in detail['fields']}:
                    raise ValueError('unknown_field')
                for field in detail['fields']:
                    if keys and field['key'] not in keys:
                        continue
                    result['total'] += 1
                    if field['status'] in ('pending', 'running'):
                        result['active'] += 1
                    elif not force and (field['translation'] or field['status'] == 'unknown'):
                        result['existing'] += 1
                    elif force:
                        result['fresh'] += 1
                    else:
                        signature, state, _ = self.memory_lookup(c, context, field, locale, cache)
                        active = c.execute("SELECT id FROM localization_admin.tasks WHERE memory_key=%s AND status IN ('pending','running') LIMIT 1", (signature,)).fetchone()
                        if state == 'reuse':
                            result['reusable'] += 1
                        elif state == 'review':
                            result['review'] += 1
                        elif signature in seen or active:
                            result['waiting'] += 1
                        else:
                            seen.add(signature)
                            result['fresh'] += 1
        return result

    def reuse_task(self, task):
        from .admin_store import plan
        if not task.get('memory_key'):
            return False  # Explicit regeneration always bypasses memory.
        with self.connect() as c:
            c.execute('SELECT localization.lock_source()')
            record=c.execute('SELECT * FROM localization_admin.segments WHERE id=%s FOR UPDATE',(task['segment_id'],)).fetchone()
            if record['revision']!=task['expected_revision']:
                c.execute("UPDATE localization_admin.tasks SET status='skipped',error_code='manual_edit_preserved' WHERE id=%s",(task['id'],))
                return True
            context=self.context(str(task['lesson_id']),c)
            field=next((f for f in plan(context)['fields'] if f['key']==task['field_key'] and f['source_hash']==task['source_hash']),None)
            if not field or memory_key(context,field,task['locale'])!=task['memory_key']:
                raise ValueError('source_changed_reload')
            _,state,source=self.memory_lookup(c,context,field,task['locale'])
            if state=='fresh':
                return False
            if state=='reuse':
                self.reuse(c,context,field,task['locale'],record,source,'worker')
                c.execute("UPDATE localization_admin.tasks SET status='reused',updated_at=now() WHERE id=%s",(task['id'],))
            else:
                c.execute("UPDATE localization_admin.tasks SET status='failed',error_code='translation_memory_requires_review',updated_at=now() WHERE id=%s",(task['id'],))
                c.execute("UPDATE localization_admin.segments SET status='failed',error_code='translation_memory_requires_review' WHERE id=%s",(record['id'],))
            return True

    def resolve_waiting(self):
        from .admin_store import plan
        with self.connect() as c:
            c.execute('SELECT localization.lock_source()')
            c.execute('SELECT pg_advisory_xact_lock(172901324)')
            rows = c.execute("SELECT t.*,s.lesson_id,s.field_key,s.source_hash,s.locale FROM localization_admin.tasks t JOIN localization_admin.tasks d ON d.id=t.depends_on JOIN localization_admin.runs r ON r.id=t.run_id JOIN localization_admin.segments s ON s.id=t.segment_id WHERE t.status='waiting' AND r.status='running' AND d.status NOT IN ('pending','running','waiting') ORDER BY t.created_at,t.id LIMIT 50 FOR UPDATE OF t SKIP LOCKED").fetchall()
            for task in rows:
                record = c.execute('SELECT * FROM localization_admin.segments WHERE id=%s FOR UPDATE', (task['segment_id'],)).fetchone()
                code, status = 'shared_translation_requires_review', 'failed'
                if record['revision'] != task['expected_revision']:
                    code, status = 'manual_edit_preserved', 'skipped'
                else:
                    context = self.context(str(task['lesson_id']), c)
                    field = next((f for f in plan(context)['fields'] if f['key'] == task['field_key'] and f['source_hash'] == task['source_hash']), None)
                    if not field or memory_key(context, field, task['locale']) != task['memory_key']:
                        code = 'source_changed_reload'
                    else:
                        _, state, source = self.memory_lookup(c, context, field, task['locale'])
                        if state == 'reuse':
                            self.reuse(c, context, field, task['locale'], record, source, 'worker')
                            code, status = None, 'reused'
                c.execute('UPDATE localization_admin.tasks SET status=%s,error_code=%s,updated_at=now() WHERE id=%s', (status, code, task['id']))
                if status == 'failed':
                    c.execute("UPDATE localization_admin.segments SET status='failed',error_code=%s,updated_at=now() WHERE id=%s AND revision=%s", (code, record['id'], task['expected_revision']))
