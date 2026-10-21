from copy import deepcopy
import pytest
from localizer.content import plan, digest
from localizer.validation import Bundle,Review,validate


def candidate(mapping,locale='fa'):
    return Bundle(locale=locale,context_hash=mapping['context_hash'],translations=[{'key':f['key'],'text':'ترجمه '+f['text'],'supplementary_note':'','issues':[]} for f in mapping['fields']])


def reviewed(mapping):
    return Review(findings=[dict(key=f['key'],meaning='pass',completeness='pass',terminology='pass',language='pass',pedagogy='pass',issues=[]) for f in mapping['fields']])


def test_real_lesson_preserves_assessment_and_relations(context):
    original=deepcopy(context); mapping=plan(context)
    assert len(mapping['fields'])>=10
    assert context==original
    for field in mapping['fields']:
        assert not any(word in field['path'] for word in ('answer_key','options','forms','prompt.text','statement'))
        if field['path']=='prompt.question':
            assert '____' not in field['text']
    assert any(i['reason']=='graded_statement_requires_human_mapping' for i in mapping['ambiguous'])


def test_source_answer_and_option_order_change_invalidate_context(context):
    before=plan(context)['context_hash']
    context['exercises'][1]['prompt']['options'].reverse()
    assert plan(context)['context_hash']!=before

@pytest.mark.parametrize('source,target',[('Hello {{name}}','سلام {{other}}'),('Pick <b>one</b>','یکی را برگزین'),('Fill ____','پر کن'),('Count %s','شمارش %d'),('Use "speaks"','از فعل استفاده کن')])
def test_protected_tokens(source,target,context):
    mapping=plan(context); mapping['fields']=mapping['fields'][:1]; mapping['fields'][0]['text']=source
    bundle=candidate(mapping); bundle.translations[0].text=target
    with pytest.raises(ValueError): validate(bundle,reviewed(mapping),mapping,'fa')


def test_duplicate_omission_and_wrong_language_rejected(context):
    mapping=plan(context); bundle=candidate(mapping)
    validate(bundle,reviewed(mapping),mapping,'fa')
    bundle.translations.append(bundle.translations[0])
    with pytest.raises(ValueError): validate(bundle,reviewed(mapping),mapping,'fa')
    bundle=candidate(mapping); bundle.translations[0].text='English only'
    with pytest.raises(ValueError): validate(bundle,reviewed(mapping),mapping,'fa')


def test_sorani_requires_native_review_and_cannot_approve(context):
    mapping=plan(context); bundle=candidate(mapping,'ckb')
    result=validate(bundle,reviewed(mapping),mapping,'ckb')
    assert all('native_sorani_review_required' in v for v in result.values())
    with pytest.raises(ValueError): Bundle.model_validate({**bundle.model_dump(),'status':'human_approved'})


def test_unicode_bidi_injection_rejected(context):
    mapping=plan(context); bundle=candidate(mapping)
    bundle.translations[0].text+='\u202e'
    with pytest.raises(ValueError): validate(bundle,reviewed(mapping),mapping,'fa')


def test_all_nine_real_exercise_types_preserve_answers_and_options(context):
    import json
    from pathlib import Path
    samples=json.loads((Path(__file__).parent/'fixtures/real-exercise-samples.json').read_text())
    assert {r['type'] for r in samples}=={'fill_blank','fill_gaps','listen_choose','match','mcq','select_words','sentence_builder','speak','true_false'}
    context['exercises']=samples
    original=deepcopy(samples)
    mapping=plan(context)
    assert context['exercises']==original
    assert all(f['path'] in ('prompt.instruction','prompt.explanation','prompt.explanation.en','prompt.question') for f in mapping['fields'] if f['source_type']=='exercise')
