"""Conservative allowlist derived from the deployed Busuu importer and real rows."""
import hashlib
import json
import re
from dataclasses import asdict, dataclass


def digest(value):
    return hashlib.sha256(json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':'), default=str).encode()).hexdigest()


@dataclass(frozen=True)
class Field:
    source_type: str
    source_id: str
    path: str
    text: str

    @property
    def key(self):
        return f'{self.source_type}:{self.source_id}:{self.path}'

    def data(self):
        return {**asdict(self), 'key': self.key, 'source_hash': digest(self.text)}


def english(value):
    return value.get('en') if isinstance(value, dict) else value


def plan(context):
    fields, ambiguous = [], []
    source_language = context['course']['language']

    def add(kind, row, path, value):
        text = value.get(source_language) if isinstance(value, dict) else value
        if isinstance(text, str) and text.strip():
            fields.append(Field(kind, str(row['id']), path + ('.'+source_language if isinstance(value, dict) else ''), text))

    lesson = context['lesson']
    add('lesson', lesson, 'title', lesson.get('title'))
    objective = lesson.get('objective') or {}
    for name in ('summary', 'tip'):
        add('lesson', lesson, f'objective.{name}', objective.get(name))
    for index, point in enumerate(objective.get('points', [])):
        add('lesson', lesson, f'objective.points.{index}', point)
    for row in context.get('items', []):
        payload = row.get('payload') or {}
        if row['kind'] == 'grammar' and payload.get('source_lang', source_language) == source_language:
            for name in ('title', 'rule'):
                add('lesson_item', row, f'payload.{name}', payload.get(name))
        elif row['kind'] == 'text' and isinstance(payload.get('phrase'), str):
            # Separate meaning field: the taught phrase is never replaced.
            add('lesson_item', row, 'payload.translation', payload['phrase'])
        elif row['kind'] not in ('exercise', 'grammar'):
            ambiguous.append({'source_id': str(row['id']), 'reason': 'unmapped_item_content_preserved'})
    for row in context.get('exercises', []):
        prompt = row['prompt']
        for name in ('instruction', 'explanation'):
            add('exercise', row, f'prompt.{name}', prompt.get(name))
        if 'question' in prompt:
            question = prompt['question'].get(source_language) if isinstance(prompt['question'], dict) else prompt['question']
            if isinstance(question, str) and re.match(r'^(Choose|Select|Listen and choose|Which (?:word|sentence|option))\b', question) and not re.search(r'_{2,}|\{[^}]*\}', question):
                add('exercise', row, 'prompt.question', prompt['question'])
            else:
                ambiguous.append({'source_id': str(row['id']), 'reason': 'question_may_be_target_english_preserved'})
        if 'statement' in prompt:
            ambiguous.append({'source_id': str(row['id']), 'reason': 'graded_statement_requires_human_mapping'})
    return {'context_hash': digest(context), 'fields': [f.data() for f in fields], 'ambiguous': ambiguous,
            'preserved': ['answer_key', 'options', 'text', 'words', 'segments', 'left', 'right', 'forms', 'media', 'phrase', 'ids', 'order']}
