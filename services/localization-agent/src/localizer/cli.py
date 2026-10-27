import argparse
import json
import os
import sys
from pathlib import Path
from uuid import UUID
import tiktoken
from .content import plan
from .hosted import Hosted, HostedError, PRICES
from .resources import rules, versions
from .store import Store
from .validation import Bundle, Review


def emit(value):
    print(json.dumps(value,ensure_ascii=False,indent=2,default=str))


def estimate(context):
    mapping=plan(context)
    # Explicit proxy tokenizer, not an asserted tokenizer for unverified models.
    enc=tiktoken.get_encoding('o200k_base')
    count=lambda x: len(enc.encode(json.dumps(x,ensure_ascii=False,default=str)))
    base=count(context)+count(mapping)+count(rules())+count(Bundle.model_json_schema())+count(Review.model_json_schema())
    output=max(2000,sum(count(f['text']) for f in mapping['fields'])*3)
    # Two languages, separate translation/review sessions, tool schemas/repeated context,
    # and generous allowance for reasoning. Not an API-enforced maximum.
    inp=base*12
    out=(output+8000)*4
    return {'dry_run':True,'writes':0,'billable_requests':0,'field_count':len(mapping['fields']),
            'ambiguous':mapping['ambiguous'],'context_hash':mapping['context_hash'],
            'tokenizer_proxy':'o200k_base','estimated_input_tokens':inp,'estimated_output_including_reasoning_tokens':out,
            'pricing_date':'2026-10-21','estimated_usd':{m:round((inp*p[0]+out*p[1])/1e6,4) for m,p in PRICES.items()},
            'suggested_pilot_ceiling_usd':5,'budget_control':'best-effort polling/cancellation; not a provider hard dollar cap'}


def worker(store, run_id, hosted, max_jobs):
    run=store.status(run_id)
    # Fixed phase allocation; never borrow budget from other jobs or failed attempts.
    allowance=float(run['run']['budget_usd'])/max(1,len(run['jobs'])*2)
    for _ in range(max_jobs):
        job=store.claim(run_id)
        if job is None: break
        try:
            translated=hosted.phase(job,'translate',Bundle.model_json_schema(),{},store,allowance)
            bundle=Bundle.model_validate(translated)
            reviewed=hosted.phase(job,'review',Review.model_json_schema(),{'candidate':bundle.model_dump()},store,allowance)
            result=store.save(job['id'],job['claim_token'],bundle,Review.model_validate(reviewed))
            emit({'job_id':job['id'],**result})
        except HostedError as e:
            code=str(e)
            if code=='run_paused': break
            store.fail(job['id'],job['claim_token'],code)
            emit({'job_id':job['id'],'status':'failed','error_code':code})
        except Exception:
            store.fail(job['id'],job['claim_token'],'validation_or_database_failed')
            emit({'job_id':job['id'],'status':'failed','error_code':'validation_or_database_failed'})
    # Update completed/failed run state when the last bounded batch finishes.
    status=store.status(run_id)
    if not any(j['status'] in ('pending','claimed') for j in status['jobs']) and status['run']['status']=='running':
        store.claim(run_id)
    emit(store.status(run_id))


def main():
    parser=argparse.ArgumentParser(description='Independent localization control plane; default is dry-run')
    sub=parser.add_subparsers(dest='command',required=True)
    for command in ('dry-run','start'):
        p=sub.add_parser(command)
        p.add_argument('--fixture',type=Path)
        p.add_argument('--lesson')
        if command=='start':
            p.add_argument('--execute',action='store_true')
            p.add_argument('--budget-usd',type=float,default=0)
            p.add_argument('--model')
            p.add_argument('--review-model')
            p.add_argument('--accept-soft-budget',action='store_true',help='Acknowledge usage polling cannot enforce a provider hard cap')
    sub.add_parser('check-openai')
    sub.add_parser('serve')
    for command in ('status','pause','resume','work','export','refresh'):
        p=sub.add_parser(command); p.add_argument('run_id')
        if command in ('work','resume'): p.add_argument('--max-jobs',type=int,default=2)
    args=parser.parse_args()
    try:
        if args.command=='serve':
            import uvicorn
            uvicorn.run('localizer.server:app',factory=True,host='0.0.0.0',port=8000,access_log=False,log_level='critical')
            return
        if args.command=='check-openai':
            emit(Hosted().access()); return
        if args.command in ('dry-run','start'):
            if args.fixture:
                context=json.loads(args.fixture.read_text())
            elif args.lesson:
                context=Store().context(str(UUID(args.lesson)))
            else:
                parser.error('--fixture or --lesson is required')
            report=estimate(context)
            emit(report)
            if args.command=='dry-run' or not args.execute: return
            if args.fixture: raise ValueError('paid_execution_requires_live_lesson')
            approved=float(os.environ.get('APPROVED_BUDGET_USD','0'))
            if not 0<args.budget_usd<=approved: raise ValueError('explicit_approved_budget_required')
            if not args.accept_soft_budget: raise ValueError('provider_hard_cap_not_available_acknowledge_soft_budget')
            model=args.model or os.environ.get('OPENAI_MODEL')
            review_model=args.review_model or model
            access=Hosted().access()
            if model not in access['candidate_models'] or review_model not in access['candidate_models']:
                raise ValueError('requested_models_not_verified_in_account')
            if max(report['estimated_usd'][model],report['estimated_usd'][review_model])>args.budget_usd:
                raise ValueError('pilot_estimate_exceeds_approved_budget')
            run=Store().enqueue([args.lesson],model,review_model,args.budget_usd,authorization_limit=approved)
            emit({'run_id':run,'status':'queued','next':f'l10n work {run}'})
            return
        UUID(args.run_id)
        store=Store()
        if args.command=='status': emit(store.status(args.run_id))
        elif args.command=='export':
            store.refresh(args.run_id)
            emit(store.export(args.run_id))
        elif args.command=='refresh': emit(store.refresh(args.run_id))
        elif args.command=='pause':
            store.control(args.run_id,'paused')
            # Cancel known active sessions; persist results and billing records for reconciliation.
            for job in store.status(args.run_id)['jobs']:
                if job['status']=='claimed':
                    for key in ('session_id','review_session_id'):
                        if job[key]: Hosted().cancel(job[key])
            emit(store.status(args.run_id))
        elif args.command in ('work','resume'):
            if not 1<=args.max_jobs<=50: raise ValueError('batch_out_of_range')
            if args.command=='resume': store.control(args.run_id,'running')
            worker(store,args.run_id,Hosted(),args.max_jobs)
    except Exception as e:
        # Do not print exception bodies: HTTP/database errors may contain credentials or source text.
        code=str(e) if isinstance(e,(ValueError,HostedError)) and len(str(e))<100 and all(c.isalnum() or c=='_' for c in str(e)) else type(e).__name__
        emit({'status':'error','error_code':code})
        sys.exit(1)

if __name__=='__main__': main()
