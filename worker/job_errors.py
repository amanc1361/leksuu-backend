"""Stable, non-sensitive job failure codes for the admin UI."""
import re


def failure_code(error, operation):
    if isinstance(error, TimeoutError):
        return "provider_timeout"
    if isinstance(error, ConnectionError):
        return "provider_unavailable"
    if isinstance(error, MemoryError):
        return "out_of_memory"
    if isinstance(error, ImportError):
        return ("translation_dependency_unavailable" if operation == "translate"
                else "processing_dependency_unavailable")
    code = str(error) if isinstance(error, ValueError) else type(error).__name__
    return code if re.fullmatch(r"[a-z0-9_]+", code) else "processing_failed"
