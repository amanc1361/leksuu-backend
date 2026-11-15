"""Opt-in cloud draft providers. No call is made without explicit billing gates.

Canonical targets remain `fa` and `ckb`. Azure's `ku` mapping is specific to
its documented Central Kurdish text-translation target, never a generic map.
"""
import html
import json
import os
import time
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import Request, urlopen

AZURE_TARGETS = {"fa": "fa", "ckb": "ku"}
GOOGLE_TARGETS = {"fa": "fa", "ckb": "ckb"}
_last_request = 0.0


def provider_name():
    name = os.environ.get("MEDIA_JOBS_TRANSLATION_PROVIDER", "local").strip().lower()
    if name not in ("local", "google", "azure"):
        raise ValueError("translation_provider_unsupported")
    return name


def cloud_job_budget(source_characters):
    if provider_name() == "local":
        return
    if os.environ.get("MEDIA_JOBS_CLOUD_TRANSLATION_ALLOWED") != "true":
        raise ValueError("cloud_translation_not_authorized")
    try:
        limit = int(os.environ.get("MEDIA_JOBS_CLOUD_MAX_CHARS_PER_JOB", "0"))
    except ValueError as error:
        raise ValueError("cloud_translation_budget_invalid") from error
    if limit <= 0 or source_characters > limit:
        raise ValueError("cloud_translation_budget_exceeded")


def translate_cloud(text, target, opener=urlopen):
    global _last_request
    name = provider_name()
    if name == "local":
        raise ValueError("cloud_translation_provider_required")
    cloud_job_budget(len(text))
    if target not in ("fa", "ckb", "en") or not text or len(text) > 4000:
        raise ValueError("cloud_translation_request_invalid")
    if name == "google":
        key = os.environ.get("LEKSUU_GOOGLE_TRANSLATE_API_KEY", "")
        if not key:
            raise ValueError("cloud_translation_credentials_unavailable")
        url = "https://translation.googleapis.com/language/translate/v2"
        body = {"q": text, "source": "en" if target != "en" else "fa",
                "target": GOOGLE_TARGETS.get(target, "en"), "format": "text"}
        headers = {"X-goog-api-key": key}
    else:
        key = os.environ.get("LEKSUU_AZURE_TRANSLATOR_KEY", "")
        if not key:
            raise ValueError("cloud_translation_credentials_unavailable")
        url = "https://api.cognitive.microsofttranslator.com/translate?" + urlencode(
            {"api-version": "3.0", "from": "en" if target != "en" else "fa",
             "to": AZURE_TARGETS.get(target, "en")})
        body = [{"Text": text}]
        headers = {"Ocp-Apim-Subscription-Key": key}
        region = os.environ.get("LEKSUU_AZURE_TRANSLATOR_REGION", "").strip()
        if region:
            headers["Ocp-Apim-Subscription-Region"] = region
    headers["Content-Type"] = "application/json; charset=utf-8"
    try:
        minimum_interval = max(0.0, float(os.environ.get("MEDIA_JOBS_CLOUD_MIN_INTERVAL_SECONDS", "0.2")))
    except ValueError as error:
        raise ValueError("cloud_translation_rate_invalid") from error
    if minimum_interval > 10:
        raise ValueError("cloud_translation_rate_invalid")
    remaining = minimum_interval - (time.monotonic() - _last_request)
    if remaining > 0:
        time.sleep(remaining)
    request = Request(url, data=json.dumps(body, ensure_ascii=False).encode("utf-8"), headers=headers,
                      method="POST")
    _last_request = time.monotonic()
    try:
        with opener(request, timeout=15) as response:
            data = json.load(response)
    except HTTPError as error:
        if error.code in (408, 429, 500, 502, 503, 504):
            raise TimeoutError("translation_provider_transient") from error
        raise ValueError("translation_provider_rejected") from error
    except (URLError, TimeoutError) as error:
        raise TimeoutError("translation_provider_transient") from error
    try:
        value = (data["data"]["translations"][0]["translatedText"] if name == "google" else
                 data[0]["translations"][0]["text"])
    except (KeyError, IndexError, TypeError) as error:
        raise ValueError("translation_provider_invalid_response") from error
    value = html.unescape(value).strip()
    if not value or len(value) > 10000:
        raise ValueError("translation_provider_invalid_response")
    return value
