"""Server-side provider settings. Secrets never appear in public settings responses."""
import ipaddress
import json
import math
import os
import re
import socket
from pathlib import Path
from urllib.parse import urlsplit
from uuid import uuid4

import httpx

from .hosted import Hosted, HostedError


def endpoint(value):
    parsed = urlsplit(value)
    if parsed.scheme != 'https' or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError('public_https_endpoint_required')
    # API keys must not be forwarded to internal services via an admin-supplied URL.
    try:
        addresses = socket.getaddrinfo(parsed.hostname, parsed.port or 443, type=socket.SOCK_STREAM)
    except OSError:
        raise ValueError('provider_hostname_unreachable') from None
    if not addresses or any(not ipaddress.ip_address(a[4][0]).is_global for a in addresses):
        raise ValueError('public_https_endpoint_required')
    return value.rstrip('/')


class Providers:
    def __init__(self, directory):
        self.directory = Path(directory)
        self.directory.mkdir(parents=True, exist_ok=True, mode=0o700)

    def all(self):
        return [json.loads(p.read_text()) for p in sorted(self.directory.glob('*.json'))]

    def public(self):
        return [{k: v for k, v in p.items() if k != 'api_key'} | {'has_key': bool(p.get('api_key'))} for p in self.all()]

    def get(self, identity):
        if not re.fullmatch(r'[0-9a-f-]{36}', identity or ''):
            raise ValueError('provider_not_found')
        path = self.directory / (identity + '.json')
        if not path.exists():
            raise ValueError('provider_not_found')
        return json.loads(path.read_text())

    def save(self, data):
        identity = data.get('id') or str(uuid4())
        old = self.get(identity) if data.get('id') else {}
        kind = data.get('kind', 'openai')
        if kind not in ('openai', 'gemini', 'compatible'):
            raise ValueError('unsupported_provider_kind')
        base = {'openai': 'https://api.openai.com/v1',
                'gemini': 'https://generativelanguage.googleapis.com/v1beta'}.get(kind, data.get('base_url', ''))
        model = str(data.get('model', '')).strip()
        if not re.fullmatch(r'[A-Za-z0-9_.:/-]{1,150}', model):
            raise ValueError('model_name_required')
        rates = [float(data.get(k, 0)) for k in ('input_rate', 'output_rate')]
        if any(not math.isfinite(v) or v <= 0 or v > 1000 for v in rates):
            raise ValueError('positive_token_prices_required')
        api_key = str(data.get('api_key') or old.get('api_key', '')).strip()
        if not api_key or len(api_key) > 4096 or '\n' in api_key or '\r' in api_key:
            raise ValueError('api_key_required')
        value = dict(id=identity, name=str(data.get('name', kind))[:120], kind=kind,
                     base_url=endpoint(base), model=model, api_key=api_key,
                     input_rate=rates[0], output_rate=rates[1])
        Hosted.persist(None, self.directory / (identity + '.json'), value)
        return next(p for p in self.public() if p['id'] == identity)


def call(provider, instruction, task, schema, max_output=4096):
    """One request, no automatic retries. Caller reserves cost before calling."""
    key = provider['api_key']
    base = endpoint(provider['base_url'])  # Recheck DNS at every request.
    headers = {'x-goog-api-key': key} if provider['kind'] == 'gemini' else {'Authorization': 'Bearer ' + key}
    with httpx.Client(timeout=120, trust_env=False, proxy=os.environ.get('LOCALIZATION_AI_PROXY'),
                      follow_redirects=False, headers=headers) as client:
        if provider['kind'] == 'gemini':
            model = provider['model'].removeprefix('models/')
            body = {'systemInstruction': {'parts': [{'text': instruction}]},
                    'contents': [{'role': 'user', 'parts': [{'text': json.dumps(task, ensure_ascii=False, default=str)}]}],
                    'generationConfig': {'maxOutputTokens': max_output, 'responseMimeType': 'application/json',
                                         'responseJsonSchema': schema}}
            url = base + '/models/' + model + ':generateContent'
        else:
            body = {'model': provider['model'], 'messages': [
                {'role': 'system', 'content': instruction},
                {'role': 'user', 'content': json.dumps(task, ensure_ascii=False, default=str)}],
                'max_completion_tokens': max_output,
                'response_format': {'type': 'json_schema', 'json_schema': {
                    'name': 'educational_translation', 'schema': schema}}}
            if provider['kind']=='compatible':
                # Generic providers commonly implement JSON mode and max_tokens,
                # while the OpenAI endpoint also supports the stricter schema API.
                body['max_tokens']=body.pop('max_completion_tokens')
                body['response_format']={'type':'json_object'}
                body['messages'][0]['content']+='\nReturn JSON matching this schema: '+json.dumps(schema)
            url = base + '/chat/completions'
        try:
            response = client.post(url, json=body)
        except httpx.TransportError:
            raise HostedError('provider_outcome_unknown') from None
        if response.status_code != 200:
            # Fixed codes only. Provider bodies can echo keys, source content or headers.
            raise HostedError('provider_http_' + str(response.status_code))
        try:
            data = response.json()
            if provider['kind'] == 'gemini':
                from .gemini import Gemini
                output = Gemini.output(data)
                u = data.get('usageMetadata') or {}
                usage = {'input_tokens': u.get('promptTokenCount', 0),
                         'output_tokens': u.get('candidatesTokenCount', 0) + u.get('thoughtsTokenCount', 0),
                         'incomplete': not bool(u)}
            else:
                choice = data['choices'][0]
                if choice.get('finish_reason') != 'stop':
                    raise HostedError('provider_incomplete_response')
                output = json.loads(choice['message']['content'])
                u = data.get('usage') or {}
                usage = {'input_tokens': u.get('prompt_tokens', 0), 'output_tokens': u.get('completion_tokens', 0),
                         'incomplete': not bool(u)}
            return output, usage
        except (KeyError, ValueError, IndexError, TypeError):
            raise HostedError('provider_invalid_response') from None
