import json

import httpx
import pytest

from localizer.gemini import Gemini, bound, payload
from localizer.hosted import HostedError


def response(output=None, finish='STOP'):
    return {'candidates': [{'finishReason': finish, 'content': {'parts': [
        {'thought': True, 'text': 'private reasoning'},
        {'text': json.dumps(output or {'ok': True})}]}}],
        'usageMetadata': {'promptTokenCount': 10, 'candidatesTokenCount': 5, 'thoughtsTokenCount': 7}}


def test_reserved_budget_and_resume_never_duplicate_inference(tmp_path):
    calls = []
    def handler(request):
        calls.append(request)
        return httpx.Response(200, json=response())
    engine = Gemini(tmp_path, 'gemini-2.5-pro', 1, httpx.Client(
        base_url='https://generativelanguage.googleapis.com', transport=httpx.MockTransport(handler)))
    assert engine.generate({'contents': []}) == {'ok': True}
    assert engine.generate({'contents': []}) == {'ok': True}
    assert len(calls) == 1
    state = json.loads(next(tmp_path.glob('*.json')).read_text())
    assert state['estimated_usd'] == (10*1.25 + 12*10)/1e6
    assert state['reserved_usd'] > state['estimated_usd']
    assert next(tmp_path.glob('*.json')).stat().st_mode & 0o777 == 0o600


def test_budget_rejection_is_before_inference(tmp_path):
    def handler(request):
        pytest.fail('must not infer')
    engine = Gemini(tmp_path, 'gemini-2.5-pro', 0.001, httpx.Client(
        base_url='https://generativelanguage.googleapis.com', transport=httpx.MockTransport(handler)))
    with pytest.raises(HostedError, match='approved_budget_exhausted'):
        engine.generate({'contents': []})


@pytest.mark.parametrize('failure', ['timeout', 'http', 'truncated', 'invalid_json'])
def test_paid_failure_is_retained_and_not_retried(tmp_path, failure):
    calls = []
    def handler(request):
        calls.append(request)
        if failure == 'timeout':
            raise httpx.ReadTimeout('secret details')
        if failure == 'http':
            return httpx.Response(403, json={'secret': 'never print'})
        data = response(finish='MAX_TOKENS' if failure == 'truncated' else 'STOP')
        if failure == 'invalid_json':
            data['candidates'][0]['content']['parts'] = [{'text': 'not json'}]
        return httpx.Response(200, json=data)
    engine = Gemini(tmp_path, 'gemini-2.5-pro', 1, httpx.Client(
        base_url='https://generativelanguage.googleapis.com', transport=httpx.MockTransport(handler)))
    for _ in range(2):
        with pytest.raises(HostedError):
            engine.generate({'contents': []})
    assert len(calls) == 1


def test_long_context_tier_refused():
    with pytest.raises(ValueError, match='input_too_large'):
        bound({'text': 'a'*200000}, 'gemini-2.5-pro')


def test_same_source_mapping_and_schema_are_used():
    from pathlib import Path
    from localizer.content import plan
    from localizer.resources import rules
    from localizer.validation import Bundle, Review
    context = json.loads((Path(__file__).parent/'fixtures/real-lesson.json').read_text())
    translated = payload(context, 'ckb', 'translate')
    data = json.loads(translated['contents'][0]['parts'][0]['text'])
    assert data['fields'] == plan(context)['fields']
    assert data['context_hash'] == plan(context)['context_hash']
    assert data['rules'] == rules()
    assert translated['generationConfig']['responseJsonSchema'] == Bundle.model_json_schema()
    reviewed = payload(context, 'ckb', 'review', {'candidate': 'example'})
    assert reviewed['generationConfig']['responseJsonSchema'] == Review.model_json_schema()
    assert 'Independently review' in reviewed['systemInstruction']['parts'][0]['text']
