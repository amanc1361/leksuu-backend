#!/usr/bin/env python3
"""Operator-only atomic import/publication of validated Gemini chapter artifacts."""
import argparse
import json
import os
from pathlib import Path
from uuid import UUID, uuid4

from psycopg.types.json import Jsonb
from localizer.content import plan, digest
from localizer.hosted import Hosted
from localizer.publication import projection
from localizer.resources import rules, versions
from localizer.store import Store
from localizer.validation import Bundle, Review, validate


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--contexts', required=True, type=Path)
    parser.add_argument('--drafts', required=True, type=Path)
    parser.add_argument('--receipt', required=True, type=Path)
    parser.add_argument('--model', required=True)
    parser.add_argument('--budget-usd', type=float, required=True)
    parser.add_argument('--execute', action='store_true')
    a = parser.parse_args()
    os.umask(0o077)
    if a.receipt.exists():
        raise ValueError('receipt_exists_reconcile_before_retry')
    contexts = json.loads(a.contexts.read_text())
    artifact = json.loads(a.drafts.read_text())
    if artifact['versions'] != list(versions()):
        raise ValueError('configuration_changed')
    if not 0 < a.budget_usd <= 5 or len(contexts) != 7:
        raise ValueError('authorized_chapter_scope_required')
    expected_lessons = {str(c['lesson']['id']) for c in contexts}
    drafts = {(str(d['lesson_id']), d['bundle']['locale']): d for d in artifact['drafts']}
    if len(drafts) != len(artifact['drafts']) or set(drafts) != {(i,l) for i in expected_lessons for l in ('fa','ckb')}:
        raise ValueError('complete_chapter_drafts_required')
    store = Store(os.environ['DATABASE_URL'])
    run_id = uuid4()
    lessons = []
    dictionary = {}
    with store.connect() as conn:
        conn.execute('SELECT pg_advisory_xact_lock(172901322)')
        conn.execute('SELECT localization.lock_source()')
        for original in contexts:
            lesson_id = str(original['lesson']['id'])
            if original['unit']['id'] != '9ad02a5c-5971-4f1a-b8eb-c4927fccb363':
                raise ValueError('wrong_chapter')
            current = store.context(lesson_id, conn)
            mapping = plan(current)
            if mapping['context_hash'] != plan(original)['context_hash']:
                raise ValueError('source_changed_before_publication')
            fields = {f['key']: f for f in mapping['fields']}
            records = []
            jobs = []
            for locale in ('fa','ckb'):
                candidate = drafts[(lesson_id, locale)]
                bundle = Bundle.model_validate(candidate['bundle'])
                review = Review.model_validate(candidate['review'])
                issues = validate(bundle, review, mapping, locale, rules()['glossary'])
                job_id = uuid4()
                jobs.append((job_id, locale))
                for translation in bundle.translations:
                    field = fields[translation.key]
                    records.append(dict(job_id=job_id, source_type=field['source_type'],
                        source_id=field['source_id'], field_path=field['path'], locale=locale,
                        source_hash=field['source_hash'], context_hash=bundle.context_hash,
                        source_text=field['text'], translation=translation.text,
                        supplementary_note=translation.supplementary_note,
                        issues=issues[translation.key], review=review.model_dump(),
                        status='needs_human_review'))
            after, entries, changes = projection(current, records)
            for entry in entries:
                key = (entry['source_hash'], entry['locale'])
                if key in dictionary and dictionary[key]['text'] != entry['text']:
                    raise ValueError('conflicting_shared_translation')
                dictionary[key] = entry
            lessons.append(dict(lesson_id=lesson_id, before=current, after=after,
                                projected_source_hash=plan(after)['context_hash'],
                                records=records, jobs=jobs, changes=changes))
        existing = []
        for key, entry in dictionary.items():
            old = conn.execute('SELECT * FROM public.content_translations WHERE source_hash=%s AND locale=%s FOR UPDATE', key).fetchone()
            if old:
                existing.append(old)
                if old['text'] != entry['text']:
                    raise ValueError('existing_dictionary_translation_conflict')
        receipt = dict(status='prepared' if a.execute else 'dry_run', run_id=str(run_id),
            model=a.model, owner_authorized_publication=True, native_review_verified=False,
            artifact_hash=digest(artifact), dictionary_before=existing,
            dictionary_after=list(dictionary.values()), lessons=lessons)
        receipt = json.loads(json.dumps(receipt, ensure_ascii=False, default=str))
        a.receipt.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        # Write ahead of the transaction commit. An ambiguous commit must be reconciled.
        fd = os.open(a.receipt, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as output:
            json.dump(receipt, output, ensure_ascii=False, default=str)
            output.flush(); os.fsync(output.fileno())
        if a.execute:
            conn.execute("INSERT INTO localization.runs(id,status,model,review_model,instruction_version,glossary_version,budget_usd) VALUES (%s,'completed',%s,%s,%s,%s,%s)",
                         (run_id, a.model, a.model, *versions(), a.budget_usd))
            for lesson in lessons:
                for job_id, locale in lesson['jobs']:
                    conn.execute("INSERT INTO localization.jobs(id,run_id,lesson_id,locale,source_hash,status) VALUES (%s,%s,%s,%s,%s,'draft')",
                                 (job_id, run_id, UUID(lesson['lesson_id']), locale, plan(lesson['before'])['context_hash']))
                for record in lesson['records']:
                    conn.execute("INSERT INTO localization.drafts(id,job_id,source_type,source_id,field_path,locale,source_hash,context_hash,source_text,translation,supplementary_note,issues,review,model,review_model,instruction_version,glossary_version) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)",
                        (uuid4(),record['job_id'],record['source_type'],UUID(record['source_id']),record['field_path'],record['locale'],record['source_hash'],record['context_hash'],record['source_text'],record['translation'],record['supplementary_note'],Jsonb(record['issues']),Jsonb(record['review']),a.model,a.model,*versions()))
                after = lesson['after']
                conn.execute('UPDATE public.lessons SET objective=%s WHERE id=%s', (Jsonb(after['lesson']['objective']),UUID(lesson['lesson_id'])))
                for exercise in after['exercises']:
                    conn.execute('UPDATE public.exercises SET prompt=%s WHERE id=%s', (Jsonb(exercise['prompt']),UUID(exercise['id'])))
                if store.context(lesson['lesson_id'], conn) != after:
                    raise ValueError('unexpected_source_mutation')
            for entry in dictionary.values():
                conn.execute('INSERT INTO public.content_translations(source_hash,locale,source_lang,source_text,text,origin) VALUES (%(source_hash)s,%(locale)s,%(source_lang)s,%(source_text)s,%(text)s,%(origin)s) ON CONFLICT(source_hash,locale) DO NOTHING', entry)
    if a.execute:
        receipt['status'] = 'committed'
        Hosted.persist(None, a.receipt, receipt)
    print(json.dumps(dict(status=receipt['status'],run_id=str(run_id),lessons=len(lessons),
                         drafts=sum(len(l['records']) for l in lessons),dictionary_entries=len(dictionary))))


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        code = str(error) if isinstance(error, ValueError) else type(error).__name__
        if not code.replace('_','').isalnum() or len(code)>100:
            code=type(error).__name__
        print(json.dumps({'status':'error','error_code':code}))
        raise SystemExit(1)
