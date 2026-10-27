"""Explicit bulk publication with actor-bound, expiring revision snapshots."""
from uuid import UUID, uuid4
from psycopg.types.json import Jsonb


class BatchPublicationMixin:
    def prepare_publication(self, lesson_ids, locale, actor):
        self.locale(locale)
        if not isinstance(lesson_ids, list) or not 1 <= len(lesson_ids) <= 100:
            raise ValueError('batch_publication_scope_required')
        identities = list(dict.fromkeys(str(UUID(i)) for i in lesson_ids))
        snapshot, summary = [], []
        with self.connect() as c:
            c.execute('SET TRANSACTION ISOLATION LEVEL REPEATABLE READ')
            for identity in identities:
                detail = self.lesson(identity, locale, c)
                drafts = [f for f in detail['fields'] if f['status'] == 'draft']
                if not drafts:
                    summary.append(dict(lesson_id=identity, title=detail['title'], fields=0, review_fields=0))
                    continue
                snapshot.append(dict(lesson_id=identity, title=detail['title'], context_hash=detail['context_hash'],
                                     keys=[f['key'] for f in drafts], revisions={f['key']: f['revision'] for f in drafts}))
                summary.append(dict(lesson_id=identity, title=detail['title'], fields=len(drafts), review_fields=sum(bool(f['issues']) for f in drafts)))
            identity = uuid4()
            c.execute('INSERT INTO localization_admin.publication_batches(id,actor,locale,snapshot) VALUES (%s,%s,%s,%s)', (identity, actor, locale, Jsonb(snapshot)))
            self.audit(c, actor, 'prepare_bulk_publication', None, {'batch_id': str(identity), 'lessons': len(snapshot)})
        return dict(batch_id=str(identity), locale=locale, lessons=summary,
                    fields=sum(l['fields'] for l in summary), review_fields=sum(l['review_fields'] for l in summary))

    def publish_batch(self, batch_id, actor, replace_shared=False):
        with self.connect() as c:
            batch = c.execute("SELECT *,created_at < now()-interval '10 minutes' expired FROM localization_admin.publication_batches WHERE id=%s FOR UPDATE", (UUID(batch_id),)).fetchone()
            if not batch or batch['actor'] != actor:
                raise ValueError('publication_batch_not_found')
            if batch['receipt'] is not None:
                return batch['receipt']
            if batch['expired']:
                raise ValueError('publication_preview_expired')
            result = dict(published_lessons=0, published_fields=0, failed=[])
            for lesson in batch['snapshot']:
                try:
                    # Each lesson is atomic. A conflict in one never hides other results.
                    current=self.lesson(lesson['lesson_id'],batch['locale'])
                    selected=[f for f in current['fields'] if f['key'] in lesson['keys']]
                    already=len(selected)==len(lesson['keys']) and all(f['status']=='published' and f['revision']==lesson['revisions'][f['key']] for f in selected)
                    if not already:
                        self.publish(lesson['lesson_id'], batch['locale'], lesson['keys'], lesson['revisions'], actor,
                                     replace_shared, expected_context_hash=lesson['context_hash'])
                    result['published_lessons'] += 1
                    result['published_fields'] += len(lesson['keys'])
                except ValueError as error:
                    code = str(error)
                    if len(code) > 100 or not code.replace('_', '').isalnum():
                        code = 'validation_failed'
                    result['failed'].append(dict(lesson_id=lesson['lesson_id'], title=lesson['title'], code=code))
                except Exception:
                    result['failed'].append(dict(lesson_id=lesson['lesson_id'], title=lesson['title'], code='admin_workspace_failed'))
            c.execute('UPDATE localization_admin.publication_batches SET receipt=%s WHERE id=%s', (Jsonb(result), batch['id']))
            self.audit(c, actor, 'bulk_publication_result', None, {'batch_id': batch_id, **result})
        return result
