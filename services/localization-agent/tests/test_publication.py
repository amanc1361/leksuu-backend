import copy
import pytest
from localizer.content import plan
from localizer.publication import projection


def drafts_for(context):
    return [dict(source_type=f['source_type'], source_id=f['source_id'], field_path=f['path'], source_text=f['text'], locale=locale, translation='translated '+locale, status='needs_human_review') for f in plan(context)['fields'] for locale in ('fa','ckb')]


def test_publication_preserves_english_answers_and_source_snapshot(context):
    before=copy.deepcopy(context)
    after,dictionary,changes=projection(context,drafts_for(context))
    assert context==before and not changes
    assert after['lesson']['title']==before['lesson']['title']
    assert after['lesson']['objective']['title']=={'en':before['lesson']['title'],'fa':'translated fa','ckb':'translated ckb'}
    assert after['items']==before['items']
    assert {t['locale'] for t in dictionary}=={'fa','ckb'}
    for old,new in zip(before['exercises'],after['exercises']):
        for k,v in old.items():
            if k!='prompt':assert new[k]==v
        for k,v in old['prompt'].items():
            if isinstance(v,str) and new['prompt'][k]!=v:
                assert new['prompt'][k]['en']==v
            elif isinstance(v,dict):
                assert all(new['prompt'][k][lang]==text for lang,text in v.items())
            else:assert new['prompt'][k]==v


def test_publication_rejects_partial_duplicate_or_stale_drafts(context):
    drafts=drafts_for(context)
    for bad in (drafts[:-1],drafts+[drafts[0]], [{**drafts[0],'source_text':'changed'}]+drafts[1:]):
        with pytest.raises(ValueError):projection(context,bad)


def test_publication_does_not_overwrite_existing_locale_and_records_edit(context):
    drafts=drafts_for(context)
    first=next(d for d in drafts if d['source_type']=='lesson_item')
    key='|'.join(str(first[k]) for k in ('source_type','source_id','field_path','locale'))
    _,dictionary,changes=projection(context,drafts,{key:'editorial correction'})
    assert changes[0]['before']=='translated '+first['locale']
    assert changes[0]['after']=='editorial correction'
    assert any(t['text']=='editorial correction' for t in dictionary)
    context['lesson']['objective']['title']={'en':context['lesson']['title'],'fa':'existing human text'}
    with pytest.raises(ValueError,match='existing_localized_text_conflict'):projection(context,drafts)
