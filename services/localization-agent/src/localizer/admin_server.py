"""Internal admin API, reached only through the Go API's admin-role guard."""
import asyncio
from contextlib import asynccontextmanager
import hmac
import json
import math
import os
import threading
from uuid import UUID

import httpx
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route

from .admin_providers import Providers, endpoint
from .admin_store import Workspace
from .admin_worker import Worker


def response(data,status=200):
    return JSONResponse(json.loads(json.dumps(data,default=str,ensure_ascii=False)),status_code=status,
                        headers={'Cache-Control':'no-store'})


def create_app(store=None,providers=None,start_worker=True):
    store=store or Workspace(os.environ['LOCALIZATION_ADMIN_DATABASE_URL'])
    providers=providers or Providers(os.environ.get('LOCALIZATION_PROVIDER_DIR','/state/providers'))
    token=os.environ['LOCALIZATION_ADMIN_TOKEN']
    if len(token)<32:
        raise ValueError('strong_admin_service_token_required')

    @asynccontextmanager
    async def lifespan(app):
        worker=Worker(store,providers)
        thread=None
        if start_worker:
            thread=threading.Thread(target=worker.loop,daemon=True)
            thread.start()
        yield
        worker.stop.set()
        if thread:
            # An in-flight call is allowed to finish; container stop has a grace period.
            await asyncio.to_thread(thread.join,125)

    async def api(request):
        if not hmac.compare_digest(request.headers.get('authorization',''), 'Bearer '+token):
            return response({'error':{'code':'admin_service_unauthorized'}},401)
        actor=request.headers.get('x-localization-actor','')
        if not actor:
            return response({'error':{'code':'admin_actor_required'}},403)
        route=request.path_params.get('path','').strip('/')
        try:
            data={}
            if request.method!='GET':
                raw=bytearray()
                async for chunk in request.stream():
                    raw.extend(chunk)
                    if len(raw)>262144:
                        return response({'error':{'code':'request_too_large'}},413)
                data=json.loads(raw or b'{}')
                if not isinstance(data,dict):
                    raise ValueError('json_object_required')
            q=request.query_params
            if route=='providers' and request.method=='GET':
                return response({'providers':await asyncio.to_thread(providers.public)})
            if route=='providers' and request.method=='POST':
                result=await asyncio.to_thread(providers.save,data)
                with store.connect() as c:
                    store.audit(c,actor,'provider_config',None,{'provider_id':result['id'],'model':result['model']})
                return response(result)
            if route.startswith('providers/') and route.endswith('/models') and request.method=='GET':
                provider=providers.get(route.split('/')[1])
                headers={'x-goog-api-key':provider['api_key']} if provider['kind']=='gemini' else {'Authorization':'Bearer '+provider['api_key']}
                async with httpx.AsyncClient(timeout=25,trust_env=False,proxy=os.environ.get('LOCALIZATION_AI_PROXY'),follow_redirects=False) as client:
                    result=await client.get(endpoint(provider['base_url'])+'/models',headers=headers)
                if result.status_code!=200:
                    raise ValueError('provider_http_'+str(result.status_code))
                payload=result.json()
                models=[m['name'].removeprefix('models/') for m in payload.get('models',[]) if 'generateContent' in m.get('supportedGenerationMethods',[])] if provider['kind']=='gemini' else [m['id'] for m in payload.get('data',[])]
                return response({'models':models,'inference_verified':False})
            if route=='catalog' and request.method=='GET':
                return response(await asyncio.to_thread(store.catalog,q.get('course_id'),q.get('unit_id'),q.get('locale','fa')))
            if route.startswith('lessons/') and request.method=='GET':
                lesson_id=str(UUID(route.split('/')[1]))
                return response(await asyncio.to_thread(store.lesson,lesson_id,q.get('locale','fa')))
            if route=='manual' and request.method=='PUT':
                return response(await asyncio.to_thread(store.manual,str(UUID(data['lesson_id'])),data['locale'],data['key'],data['source_hash'],data['text'],int(data['revision']),actor))
            if route=='queue' and request.method=='POST':
                budget=float(data['budget_usd'])
                if not math.isfinite(budget):
                    raise ValueError('finite_budget_required')
                if not isinstance(data['lesson_ids'],list) or not isinstance(data.get('keys',[]),list) or not isinstance(data.get('force',False),bool):
                    raise ValueError('invalid_scope')
                provider=providers.get(data['provider_id'])
                return response(await asyncio.to_thread(store.enqueue,data['lesson_ids'],data['locale'],provider,budget,data.get('keys',[]),data.get('force',False),actor),202)
            if route=='runs' and request.method=='GET':
                return response({'runs':await asyncio.to_thread(store.runs)})
            if route.startswith('runs/') and request.method=='PATCH':
                await asyncio.to_thread(store.control,str(UUID(route.split('/')[1])),data['status'],actor)
                return response({'ok':True})
            if route=='publish' and request.method=='POST':
                if not isinstance(data.get('replace_shared',False),bool):
                    raise ValueError('invalid_shared_confirmation')
                return response(await asyncio.to_thread(store.publish,str(UUID(data['lesson_id'])),data['locale'],data['keys'],data['revisions'],actor,data.get('replace_shared',False)))
            return response({'error':{'code':'admin_route_not_found'}},404)
        except (ValueError,KeyError,TypeError) as error:
            code=str(error) if isinstance(error,ValueError) else 'invalid_request'
            if len(code)>100 or not code.replace('_','').isalnum():
                code='invalid_request'
            return response({'error':{'code':code}},400)
        except httpx.TransportError:
            return response({'error':{'code':'provider_connection_failed'}},502)
        except Exception:
            return response({'error':{'code':'admin_workspace_failed'}},500)

    async def health(request):
        return response({'status':'ok'})

    return Starlette(routes=[Route('/healthz',health),Route('/{path:path}',api,methods=['GET','POST','PUT','PATCH'])],lifespan=lifespan)


def app():
    return create_app()
