"""Explicit operator publication into existing app fields; never an MCP capability."""
import copy
import hashlib
from .content import plan


def dictionary_hash(text):
    return hashlib.sha256(text.strip().encode()).hexdigest()


def projection(context, drafts, overrides=None):
    """Build additive localized values without mutating source or grading data."""
    out = copy.deepcopy(context)
    fields = {(f['source_type'], f['source_id'], f['path']): f for f in plan(context)['fields']}
    expected = {(kind, source, path, locale) for kind, source, path in fields for locale in ('fa', 'ckb')}
    seen = set()
    translations = {}
    changes = []
    overrides = overrides or {}
    for draft in drafts:
        key = (draft['source_type'], str(draft['source_id']), draft['field_path'], draft['locale'])
        if key not in expected or key in seen:
            raise ValueError('unexpected_or_duplicate_draft')
        seen.add(key)
        source = fields[key[:3]]['text']
        if draft['source_text'] != source or draft.get('status') == 'rejected':
            raise ValueError('draft_source_mismatch_or_rejected')
        locale = draft['locale']
        text = overrides.get('|'.join(key), draft['translation'])
        if not isinstance(text, str) or not text.strip():
            raise ValueError('empty_translation')
        if text != draft['translation']:
            changes.append({'key': list(key), 'before': draft['translation'], 'after': text})
        kind, source_id, path = key[:3]
        if kind == 'lesson_item':
            # Existing backend projects grammar and phrase meanings from this dictionary.
            h = dictionary_hash(source)
            previous = translations.get((h, locale))
            if previous and previous['text'] != text:
                raise ValueError('conflicting_shared_translation')
            translations[(h, locale)] = dict(source_hash=h, locale=locale, source_lang='en', source_text=source, text=text, origin='machine')
        else:
            if kind == 'lesson':
                row = out['lesson']
                parts = ['objective', 'title'] if path == 'title' else path.split('.')
            elif kind == 'exercise':
                row = next(r for r in out['exercises'] if r['id'] == source_id)
                parts = path.split('.')
            else:
                raise ValueError('unsupported_publication_field')
            if parts[-1] == 'en':
                parts.pop()
            for part in parts[:-1]:
                row = row[int(part)] if isinstance(row, list) else row.setdefault(part, {})
            leaf = parts[-1]
            old = row[int(leaf)] if isinstance(row, list) else row.get(leaf)
            value = {'en': source} if old is None or isinstance(old, str) else dict(old)
            if value.get('en') != source:
                raise ValueError('source_language_mismatch')
            if value.get(locale) and value[locale] != text:
                raise ValueError('existing_localized_text_conflict')
            value[locale] = text
            if isinstance(row, list):
                row[int(leaf)] = value
            else:
                row[leaf] = value
    if seen != expected:
        raise ValueError('incomplete_lesson_drafts')
    if not set(overrides) <= {'|'.join(k) for k in expected}:
        raise ValueError('unknown_editorial_override')
    return out, list(translations.values()), changes
