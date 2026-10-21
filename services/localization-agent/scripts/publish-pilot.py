#!/usr/bin/env python3
"""Dry-run by default. Requires separate operator DATABASE_URL, never the MCP role."""
import argparse
import json
import os
from pathlib import Path
from uuid import UUID
from psycopg.types.json import Jsonb
from localizer.content import plan
from localizer.publication import projection
from localizer.store import Store

p = argparse.ArgumentParser()
p.add_argument('--run', required=True, type=UUID)
p.add_argument('--lesson', required=True, type=UUID)
p.add_argument('--expected-hash', required=True)
p.add_argument('--receipt', required=True, type=Path)
p.add_argument('--overrides', type=Path)
p.add_argument('--execute', action='store_true')
a = p.parse_args()
assert not a.receipt.exists(), 'receipt_exists_reconcile_before_retry'
s = Store(os.environ['DATABASE_URL'])
overrides = json.loads(a.overrides.read_text()) if a.overrides else {}
with s.connect() as c:
    c.execute('SELECT pg_advisory_xact_lock(172901322)')
    before = s.context(a.lesson, c, lock=True)
    if plan(before)['context_hash'] != a.expected_hash:
        raise ValueError('source_changed_before_publication')
    jobs = c.execute('SELECT * FROM localization.jobs WHERE run_id=%s ORDER BY locale', (a.run,)).fetchall()
    if len(jobs) != 2 or any(j['lesson_id'] != a.lesson or j['status'] != 'draft' or j['source_hash'] != a.expected_hash for j in jobs):
        raise ValueError('single_completed_pilot_required')
    drafts = c.execute('SELECT d.* FROM localization.drafts d JOIN localization.jobs j ON j.id=d.job_id WHERE j.run_id=%s ORDER BY d.source_type,d.source_id,d.field_path,d.locale', (a.run,)).fetchall()
    after, dictionary, changes = projection(before, drafts, overrides)
    existing = []
    for t in dictionary:
        old = c.execute('SELECT * FROM public.content_translations WHERE source_hash=%s AND locale=%s FOR UPDATE', (t['source_hash'], t['locale'])).fetchone()
        if old:
            existing.append(old)
            if old['text'] != t['text']:
                raise ValueError('existing_dictionary_translation_conflict')
    receipt = dict(status='prepared' if a.execute else 'dry_run', run_id=str(a.run), lesson_id=str(a.lesson), owner_authorized_publication=True, native_review_verified=False, draft_count=len(drafts), editorial_changes=changes, before=before, dictionary_before=existing, dictionary_after=dictionary, projected_source_hash=plan(after)['context_hash'])
    # Backup exists before commit, even if the process dies after committing.
    a.receipt.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    fd = os.open(a.receipt, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(receipt, f, ensure_ascii=False, indent=2, default=str)
        f.flush(); os.fsync(f.fileno())
    if a.execute:
        for t in dictionary:
            c.execute('INSERT INTO public.content_translations(source_hash,locale,source_lang,source_text,text,origin) VALUES (%(source_hash)s,%(locale)s,%(source_lang)s,%(source_text)s,%(text)s,%(origin)s) ON CONFLICT(source_hash,locale) DO NOTHING', t)
        c.execute('UPDATE public.lessons SET objective=%s WHERE id=%s', (Jsonb(after['lesson']['objective']), a.lesson))
        for row in after['exercises']:
            c.execute('UPDATE public.exercises SET prompt=%s WHERE id=%s', (Jsonb(row['prompt']), UUID(row['id'])))
        if s.context(a.lesson, c) != after:
            raise ValueError('unexpected_source_mutation')
        # Publishing is not native review. Existing issues and human status stay intact.
        c.execute('UPDATE localization.drafts SET needs_review=true WHERE job_id IN (SELECT id FROM localization.jobs WHERE run_id=%s)', (a.run,))
if a.execute:
    receipt['status'] = 'committed'
    a.receipt.write_text(json.dumps(receipt, ensure_ascii=False, indent=2, default=str))
print(json.dumps({'status': receipt['status'], 'draft_count': len(drafts), 'dictionary_entries': len(dictionary), 'editorial_corrections': len(changes), 'original_english_and_answers_preserved': True, 'native_review_verified': False, 'receipt_path': str(a.receipt)}))
