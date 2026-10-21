import re
import unicodedata
from collections import Counter
from typing import Literal
from pydantic import BaseModel, ConfigDict, Field


class Strict(BaseModel):
    model_config = ConfigDict(extra='forbid')


class Translation(Strict):
    key: str = Field(max_length=300)
    text: str = Field(min_length=1, max_length=30000)
    supplementary_note: str = Field(max_length=5000)
    issues: list[str] = Field(max_length=30)


class Bundle(Strict):
    locale: Literal['fa', 'ckb']
    context_hash: str = Field(pattern=r'^[0-9a-f]{64}$')
    translations: list[Translation] = Field(max_length=300)


class Finding(Strict):
    key: str
    meaning: Literal['pass', 'concern']
    completeness: Literal['pass', 'concern']
    terminology: Literal['pass', 'concern']
    language: Literal['pass', 'concern']
    pedagogy: Literal['pass', 'concern']
    issues: list[str]


class Review(Strict):
    findings: list[Finding]


TOKENS = re.compile(r'\{\{[^{}]+\}\}|\$\{[^{}]+\}|\{[^{}]+\}|%\([^)]+\)[a-z]|%\d*\$?[sdif]|</?[^>]+>|_{2,}|\[\[[^\]]+\]\]')
QUOTED = re.compile(r'"([^"\n]*[A-Za-z][^"\n]*)"|`([^`\n]+)`')


def validate(bundle: Bundle, review: Review, mapping, locale, glossary=None):
    if bundle.locale != locale or bundle.context_hash != mapping['context_hash']:
        raise ValueError('wrong_locale_or_source_version')
    expected = {f['key']: f for f in mapping['fields']}
    keys = [t.key for t in bundle.translations]
    if len(keys) != len(set(keys)) or set(keys) != set(expected):
        raise ValueError('missing_duplicate_or_protected_fields')
    review_keys = [f.key for f in review.findings]
    if len(set(review_keys)) != len(review_keys) or set(review_keys) != set(expected):
        raise ValueError('incomplete_independent_review')
    issues = {}
    for t in bundle.translations:
        src = expected[t.key]['text']
        if Counter(TOKENS.findall(src)) != Counter(TOKENS.findall(t.text)):
            raise ValueError('placeholder_tag_or_blank_changed')
        if re.findall(r'</?[^>]+>',src)!=re.findall(r'</?[^>]+>',t.text):
            raise ValueError('tag_order_changed')
        for quoted in QUOTED.findall(src):
            literal = quoted[0] or quoted[1]
            if literal not in t.text:
                raise ValueError('english_grammar_literal_changed')
        if any(unicodedata.category(c) == 'Cc' and c not in '\n\t' or c in '\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069\ufeff\ufffd' for c in t.text+t.supplementary_note):
            raise ValueError('unsafe_unicode')
        if unicodedata.normalize('NFC', t.text) != t.text:
            raise ValueError('non_normalized_unicode')
        if not re.search(r'[\u0600-\u06ff]', t.text):
            raise ValueError('target_script_missing')
        notes = list(t.issues)
        for term in (glossary or {}).get('terms',[]):
            if re.search(r'(?<![A-Za-z])'+re.escape(term['en'])+r'(?![A-Za-z])',src,re.I):
                if term.get(locale) and term[locale] not in t.text:
                    notes.append('glossary_equivalent_needs_review:'+term['en'])
                if locale=='ckb' and term.get('ckb_status')!='approved':
                    notes.append('unapproved_sorani_term:'+term['en'])
        if locale == 'ckb':
            notes.append('native_sorani_review_required')
            if re.search('[كيى]', t.text):
                notes.append('arabic_codepoint_spelling_review')
        if len(t.text) < len(src)*0.2 or len(t.text) > max(200, len(src)*5):
            notes.append('possible_content_length_mismatch')
        finding = next(f for f in review.findings if f.key == t.key)
        notes.extend(finding.issues)
        notes.extend(f'reviewer_{name}_concern' for name in ('meaning','completeness','terminology','language','pedagogy') if getattr(finding, name) == 'concern')
        issues[t.key] = sorted(set(notes))
    return issues
