#!/usr/bin/env python3
"""HTML review artifact. With no real draft, target columns explicitly remain pending."""
import argparse
import html
import json
from pathlib import Path
from localizer.content import plan
p=argparse.ArgumentParser();p.add_argument('context',type=Path);p.add_argument('output',type=Path);p.add_argument('--drafts',type=Path)
a=p.parse_args();ctx=json.loads(a.context.read_text());mapping=plan(ctx)
drafts=json.loads(a.drafts.read_text()) if a.drafts else []
lookup={(d['source_type'],str(d['source_id']),d['field_path'],d['locale']):d for d in drafts}
e=lambda x:html.escape(str(x))
rows=[]
for f in mapping['fields']:
    targets=[]
    issues=[]
    for lang in ('fa','ckb'):
        draft=lookup.get((f['source_type'],f['source_id'],f['path'],lang))
        if draft:
            rendered='<div>'+e(draft['translation'])+'</div>'
            if draft.get('supplementary_note'):
                rendered+='<small>یادداشت تکمیلی: '+e(draft['supplementary_note'])+'</small>'
            rendered+='<small style="display:block">وضعیت: '+e(draft.get('status','needs_human_review'))+'</small>'
            targets.append(rendered)
        else:
            targets.append('اجرا نشده؛ در انتظار پایلوت')
        if draft: issues+=draft.get('issues',[])
    rows.append(f"<tr><td>{e(f['path'])}</td><td dir='ltr'>{e(f['text'])}</td><td>{targets[0]}</td><td lang='ckb'>{targets[1]}</td><td>{e(', '.join(issues))}</td></tr>")
status_note='ترجمهٔ پولی هنوز اجرا نشده است؛ ستون‌های مقصد در انتظار پایلوت هستند.' if not drafts else 'پیش‌نویس‌های اجراشده؛ بررسی انسانی و سورانی بومی هنوز لازم است.'
body=f"""<!doctype html><html lang='fa' dir='rtl'><meta charset='utf-8'><title>بازبینی پایلوت ترجمه</title>
<style>body{{font-family:Tahoma,sans-serif;margin:2rem;line-height:1.8}}table{{border-collapse:collapse;width:100%}}td,th{{border:1px solid #ccc;padding:.6rem;white-space:pre-wrap;vertical-align:top}}td{{unicode-bidi:plaintext}}summary{{cursor:pointer}}pre{{overflow:auto;white-space:pre-wrap}}</style>
<h1>بازبینی پایلوت: {e(ctx['lesson']['title'])}</h1><p>متن واقعی دیتابیس؛ {e(status_note)} این گزارش تأیید کیفیت یا انتشار نیست.</p>
<table><thead><tr><th>فیلد</th><th>English</th><th>فارسی</th><th>کوردیی ناوەندی</th><th>مسائل</th></tr></thead><tbody>{''.join(rows)}</tbody></table>
<h2>موارد مبهم؛ حفظ‌شده برای بررسی</h2><pre dir='ltr'>{e(json.dumps(mapping['ambiguous'],ensure_ascii=False,indent=2))}</pre>
<details><summary>زمینهٔ کامل انگلیسی، پاسخ‌ها و روابط اصلی</summary><pre dir='ltr'>{e(json.dumps(ctx,ensure_ascii=False,indent=2))}</pre></details></html>"""
a.output.write_text(body)
