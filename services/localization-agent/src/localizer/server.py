import hmac
import json
import logging
import os
import re
from functools import wraps
from pathlib import Path
from uuid import UUID
from typing import Literal
from mcp.server.fastmcp import FastMCP
from mcp.server.transport_security import TransportSecuritySettings
from starlette.responses import JSONResponse
from .content import plan
from .store import Store
from .validation import Bundle, Review

from .resources import rules


def sanitized(function):
    @wraps(function)
    def wrapped(*args, **kwargs):
        try:
            return function(*args, **kwargs)
        except Exception as error:
            code=str(error)
            if not isinstance(error,ValueError) or not re.fullmatch(r'[a-z_]{1,80}',code):
                code='tool_failed'
            raise ValueError(code) from None
    return wrapped


def create_server(store=None):
    store = store or Store()
    host = os.environ.get('MCP_ALLOWED_HOST', 'localhost')
    mcp = FastMCP('educational-localization', stateless_http=True, json_response=True,
                  max_request_body_size=262144,
                  transport_security=TransportSecuritySettings(allowed_hosts=[host, host+':*','127.0.0.1:*','localhost:*'], allowed_origins=['https://'+host]))

    @mcp.tool()
    @sanitized
    def list_lessons(locale: Literal['fa','ckb'] = 'fa', after: str | None = None, limit: int = 20) -> dict:
        """Read English lesson candidates by stable ID cursor; inspect context before planning."""
        if after: UUID(after)
        return store.needed(locale,after,limit)

    @mcp.tool()
    @sanitized
    def get_lesson_context(lesson_id: str) -> dict:
        """Read full untrusted educational context and an explicit translation allowlist."""
        context=store.context(str(UUID(lesson_id)))
        if len(json.dumps(context).encode()) > 200000:
            raise ValueError('lesson_too_large_requires_manual_split')
        return {'context':context, **plan(context)}

    @mcp.tool()
    @sanitized
    def get_rules() -> dict:
        """Read versioned educational instructions and glossary."""
        return rules()

    @mcp.tool()
    @sanitized
    def get_run_status(run_id: str) -> dict:
        """Read actual persisted job states; no write or publication."""
        return store.status(str(UUID(run_id)))

    @mcp.tool()
    @sanitized
    def save_reviewed_draft(job_id: str, claim_token: str, bundle: Bundle, review: Review) -> dict:
        """Controller-only: validate and atomically store a reviewed draft under a live claim.
        Never marks human approval. Translation sessions are not granted this tool."""
        return store.save(str(UUID(job_id)),str(UUID(claim_token)),bundle,review)

    return mcp


class Guard:
    """Authenticate before parsing body. Logs deliberately contain no request payloads."""
    def __init__(self, app, token):
        if len(token) < 32 or token.startswith('REPLACE'):
            raise ValueError('strong_mcp_token_required')
        self.app, self.token = app, token.encode()

    async def __call__(self, scope, receive, send):
        if scope['type'] != 'http':
            return await self.app(scope,receive,send)
        headers=dict(scope['headers'])
        if scope['path']=='/healthz':
            return await JSONResponse({'status':'ok'})(scope,receive,send)
        expected=b'Bearer '+self.token
        if not hmac.compare_digest(headers.get(b'authorization',b''),expected):
            return await JSONResponse({'error':'unauthorized'},401)(scope,receive,send)
        # SDK also enforces the actual streamed body size, including chunked requests.
        try: size=int(headers.get(b'content-length',b'0'))
        except ValueError: size=262145
        if size>262144:
            return await JSONResponse({'error':'request_too_large'},413)(scope,receive,send)
        return await self.app(scope,receive,send)


def app():
    # MCP SDK may log validation payloads at DEBUG/ERROR. Never enable payload logs.
    logging.getLogger('mcp').setLevel(logging.CRITICAL)
    return Guard(create_server().streamable_http_app(),os.environ['MCP_BEARER_TOKEN'])
