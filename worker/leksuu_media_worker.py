"""Durable PostgreSQL media synchronization worker for Stories, Podcasts and Films.

Run separately from the API. It writes draft review results only. No publication
or lesson-item payload mutation occurs here. One worker processes one job at a
time; PostgreSQL SKIP LOCKED permits additional bounded worker instances.
"""

import argparse
import difflib
import hashlib
import json
import os
import re
import sys
import tempfile
import threading
import time
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

sys.path.insert(0, str(Path(tempfile.gettempdir()) / "leksuu-align-pydeps"))
import av  # noqa: E402
import psycopg  # noqa: E402
from psycopg.rows import dict_row  # noqa: E402
from psycopg.types.json import Jsonb  # noqa: E402
from faster_whisper import WhisperModel  # noqa: E402

_open = av.open


def compatible_open(*args, **kwargs):
    kwargs.pop("metadata_errors", None)
    return _open(*args, **kwargs)


av.open = compatible_open
TOKEN = re.compile(r"[^\W_]+", re.UNICODE)
MODEL = None
MODEL_NAME = None
TRANSLATOR = None
TRANSLATION_TOKENIZER = None


def words(text):
    return TOKEN.findall(text.casefold())


def db():
    database_url = os.environ.get("MEDIA_JOBS_DATABASE_URL") or os.environ["DATABASE_URL"]
    return psycopg.connect(database_url, row_factory=dict_row)


def event(connection, job_id, name, **details):
    connection.execute("UPDATE media_processing_jobs SET events=events||%s::jsonb,updated_at=now() WHERE id=%s",
                       (Jsonb([{"event": name, **details}]), job_id))


def claim(worker_id):
    with db() as connection:
        connection.execute("UPDATE media_processing_jobs SET status='pending',worker_id=NULL,lease_until=NULL,"
                           "events=events||'[{\"event\":\"worker_interrupted\"}]'::jsonb "
                           "WHERE status='running' AND lease_until<now() AND attempts<3")
        row = connection.execute("""SELECT j.id,j.item_id,j.kind,j.operation,j.target_language,
            j.payload_snapshot,j.payload_sha256,j.result
            FROM media_processing_jobs j JOIN lesson_items i ON i.id=j.item_id
            WHERE j.status='pending' AND j.attempts<3 AND NOT j.cancel_requested
            ORDER BY j.created_at,j.id FOR UPDATE OF j SKIP LOCKED LIMIT 1""").fetchone()
        if not row:
            return None
        connection.execute("""UPDATE media_processing_jobs SET status='running',attempts=attempts+1,
            worker_id=%s,lease_until=now()+interval '5 minutes',progress=5,updated_at=now()
            WHERE id=%s""", (worker_id, row["id"]))
        event(connection, row["id"], "claimed", worker=worker_id)
        return row


def heartbeat(job_id, worker_id, stop):
    while not stop.wait(20):
        try:
            with db() as connection:
                connection.execute("""UPDATE media_processing_jobs SET lease_until=now()+interval '5 minutes',
                    updated_at=now() WHERE id=%s AND worker_id=%s AND status='running'""", (job_id, worker_id))
        except Exception:
            pass


def status(job_id):
    with db() as connection:
        return connection.execute("SELECT status,cancel_requested FROM media_processing_jobs WHERE id=%s", (job_id,)).fetchone()


def fetch_original(url, folder):
    base = os.environ["MEDIA_JOBS_MEDIA_BASE_URL"].rstrip("/")
    parsed = urllib.parse.urlparse(url)
    trusted = urllib.parse.urlparse(base)
    public = urllib.parse.urlparse(os.environ.get("MEDIA_JOBS_PUBLIC_ORIGIN", ""))
    if parsed.scheme and (parsed.scheme, parsed.netloc) not in {
            (trusted.scheme, trusted.netloc), (public.scheme, public.netloc)}:
        raise ValueError("media_url_outside_configured_origin")
    path = parsed.path if parsed.scheme else url
    if not path.startswith("/media/") or ".." in path.split("/"):
        raise ValueError("media_url_not_local_asset")
    request = urllib.request.Request(base + path, headers={"User-Agent": "LeksuuMediaWorker/1"})
    media = Path(folder) / ("original" + (Path(path).suffix or ".bin"))
    maximum = int(os.environ.get("MEDIA_JOBS_MAX_BYTES", str(2 << 30)))
    if maximum <= 0:
        raise ValueError("invalid_media_size_limit")
    digest = hashlib.sha256()
    size = 0
    with urllib.request.urlopen(request, timeout=60) as response:
        if response.status != 200:
            raise ValueError("media_download_failed")
        with media.open("wb") as destination:
            while chunk := response.read(1 << 20):
                size += len(chunk)
                if size > maximum:
                    raise ValueError("media_too_large")
                digest.update(chunk)
                destination.write(chunk)
    if not size:
        raise ValueError("media_empty")
    return media, digest.hexdigest()


def audio_duration(path):
    with av.open(str(path)) as container:
        if not container.duration:
            raise ValueError("media_duration_missing")
        return container.duration / 1_000_000


def opening_sound_onset(path, threshold=0.015, maximum_seconds=5.0):
    """Find sustained opening sound; used only to reject impossible ASR starts."""
    import numpy as np
    with av.open(str(path)) as container:
        stream = next((part for part in container.streams if part.type == "audio"), None)
        if stream is None:
            return None
        resampler = av.AudioResampler(format="fltp", layout="mono", rate=16000)
        samples = []
        for frame in container.decode(stream):
            for converted in resampler.resample(frame):
                samples.append(converted.to_ndarray().reshape(-1))
            if sum(len(part) for part in samples) >= maximum_seconds * 16000:
                break
    if not samples:
        return None
    signal = np.concatenate(samples)[:int(maximum_seconds * 16000)]
    window = 1600  # 100 ms; two consecutive windows reject isolated clicks.
    for index in range(0, len(signal) - 2 * window, window):
        first = float(np.sqrt(np.mean(signal[index:index + window] ** 2)))
        second = float(np.sqrt(np.mean(signal[index + window:index + 2 * window] ** 2)))
        if first >= threshold and second >= threshold:
            return round(index / 16000, 3)
    return None


def transcribe(path, language):
    global MODEL, MODEL_NAME
    model_name = "base.en" if language == "en" else "base"
    if MODEL_NAME != model_name:
        model_root = os.environ.get("MEDIA_JOBS_MODEL_DIR",
                                str(Path(tempfile.gettempdir()) / "leksuu-whisper-models"))
        options = {"device": "cpu", "compute_type": "int8", "cpu_threads": 4,
                   "download_root": model_root}
        try:
            MODEL = WhisperModel(model_name, local_files_only=True, **options)
        except (OSError, RuntimeError, ValueError):
            MODEL = WhisperModel(model_name, local_files_only=False, **options)
        MODEL_NAME = model_name
    whisper_lang = {"ckb": "ku"}.get(language, language)
    segments, info = MODEL.transcribe(str(path), language=whisper_lang, beam_size=5,
                                      word_timestamps=True, vad_filter=True,
                                      condition_on_previous_text=False)
    return {"language": language, "duration_seconds": info.duration, "segments": [
        {"start": part.start, "end": part.end, "text": part.text,
         "words": [{"start": word.start, "end": word.end, "word": word.word,
                    "probability": word.probability} for word in (part.words or [])]}
        for part in segments]}


def translation_source(payload):
    return {"language": payload.get("language", ""), "source_id": payload.get("source_id", ""),
            "url": payload.get("url", ""), "audio_sha256": payload.get("audio_sha256", ""),
            "transcript": [cue.get("text", "") for cue in (payload.get("transcript") or [])]}


def translation_model():
    global TRANSLATOR, TRANSLATION_TOKENIZER
    if TRANSLATOR is None:
        root = Path(os.environ.get("MEDIA_JOBS_TRANSLATION_MODEL_DIR", ""))
        if not root.is_dir() or not (root / "model.bin").is_file() or not (root / "spiece.model").is_file():
            raise ValueError("translation_model_unavailable")
        import ctranslate2
        import sentencepiece
        TRANSLATOR = ctranslate2.Translator(str(root), device="cpu", compute_type="int8",
                                            inter_threads=1, intra_threads=4)
        TRANSLATION_TOKENIZER = sentencepiece.SentencePieceProcessor(model_file=str(root / "spiece.model"))
    return TRANSLATOR, TRANSLATION_TOKENIZER


def translation_chunks(text):
    if len(text.split()) <= 110:
        return [text]
    pieces = re.split(r'(?<=[.!?])\s+(?=["“‘A-Z])', text)
    chunks, current = [], ""
    for piece in pieces:
        if current and len((current + " " + piece).split()) > 110:
            chunks.append(current)
            current = piece
        else:
            current = (current + " " + piece).strip()
    if current:
        chunks.append(current)
    return chunks


def translate_text(text, target):
    translator, tokenizer = translation_model()
    if tokenizer.piece_to_id(f"<2{target}>") == tokenizer.unk_id():
        raise ValueError("unsupported_translation_target")
    output = []
    for chunk in translation_chunks(text):
        tokens = tokenizer.encode(f"<2{target}> {chunk}", out_type=str)
        if not tokens or len(tokens) > 512:
            raise ValueError("translation_source_too_long")
        result = translator.translate_batch([tokens], beam_size=2, repetition_penalty=1.15,
                    no_repeat_ngram_size=4, max_decoding_length=max(32, min(384, len(tokens) * 2 + 16)))
        pieces = [piece for piece in result[0].hypotheses[0] if piece not in ("</s>", "<pad>")]
        output.append(tokenizer.decode(pieces).strip())
    value = " ".join(output).strip()
    if not value or len(value) > 10000:
        raise ValueError("translation_output_invalid")
    if target in ("fa", "ckb") and sum(0x0600 <= ord(c) <= 0x08FF for c in value) < 2:
        raise ValueError("translation_wrong_script")
    return value


def process_translation(job, current):
    snapshot = job["payload_snapshot"]
    if translation_source(snapshot) != translation_source(current):
        raise ValueError("source_changed_since_queue")
    target = job["target_language"]
    allowed = [part.strip() for part in os.environ.get("MEDIA_TRANSLATION_TARGETS", "fa,ckb").split(",")]
    if target not in allowed or not re.fullmatch(r"[a-z]{2,3}", target):
        raise ValueError("unsupported_translation_target")
    cues = current.get("transcript") or []
    if not cues or len(cues) > int(os.environ.get("MEDIA_TRANSLATION_MAX_CUES", "120")) or \
            sum(len(cue.get("text", "")) for cue in cues) > int(os.environ.get("MEDIA_TRANSLATION_MAX_CHARS", "100000")):
        raise ValueError("translation_cost_limit")
    if current.get("translation_status", {}).get(target) in ("approved", "human_approved"):
        raise ValueError("translation_already_approved")
    checkpoint = job.get("result") or {}
    if checkpoint.get("phase") == "translation_checkpoint" and checkpoint.get("source_sha256") == job["payload_sha256"] and \
            checkpoint.get("target_language") == target:
        translated = list(checkpoint.get("translations") or [])
    else:
        translated = []
    done = {row["paragraph"] for row in translated}
    for index, cue in enumerate(cues, 1):
        if index in done or (cue.get("translation") or {}).get(target, "").strip():
            continue
        if not cue.get("text", "").strip():
            raise ValueError("translation_source_missing")
        translated.append({"paragraph": index, "text": cue["text"],
                           "translation": translate_text(cue["text"], target)})
        with db() as connection:
            connection.execute("""UPDATE media_processing_jobs SET result=%s,progress=%s,updated_at=now()
                WHERE id=%s AND status='running'""", (Jsonb({"phase": "translation_checkpoint",
                    "source_sha256": job["payload_sha256"], "target_language": target,
                    "translations": translated}), min(95, 5 + 90 * index // len(cues)), job["id"]))
            event(connection, job["id"], "translation_checkpoint_saved", paragraph=index)
    return {"source_sha256": job["payload_sha256"], "target_language": target,
            "translations": translated, "generated": len(translated),
            "model": "madlad400-3b-mt-ct2-int8", "human_editorial_approved": False}


def align(transcript, witness, duration, offset, audible_onset=None):
    source = [(word, i) for i, cue in enumerate(transcript) for word in words(cue["text"])]
    spoken = [(token, word["start"] + offset, word["end"] + offset)
              for segment in witness["segments"] for word in segment["words"]
              for token in words(word["word"])]
    matcher = difflib.SequenceMatcher(None, [word[0] for word in source],
                                      [word[0] for word in spoken], autojunk=False)
    matched = {block.a + j: block.b + j for block in matcher.get_matching_blocks() for j in range(block.size)}
    cues = []
    exceptions = []
    previous_end = 0.0
    for i, paragraph in enumerate(transcript):
        positions = [j for j, (_, owner) in enumerate(source) if owner == i]
        pairs = [(j, matched[j]) for j in positions if j in matched]
        count = len(positions)
        first = spoken[pairs[0][1]][1] if pairs else None
        last = spoken[pairs[-1][1]][2] if pairs else None
        lead = pairs[0][0] - positions[0] if pairs else count
        trail = positions[-1] - pairs[-1][0] if pairs else count
        reasons = []
        if len(pairs) < 3 or (count and len(pairs) / count < 0.8):
            reasons.append("low_word_match")
        if lead > 1 or trail > 1:
            reasons.append("unmatched_boundary_words")
        if first is None or last is None or not (previous_end <= first < last <= duration + offset + 0.3):
            reasons.append("invalid_boundary")
        if i == 0 and first is not None and audible_onset is not None and first < audible_onset + offset - 0.4:
            reasons.append("asr_boundary_before_audible_audio")
        cue = {"text": paragraph["text"]}
        if reasons:
            cue["timing_status"] = "unverified"
            exceptions.append({"paragraph": i + 1, "reasons": reasons})
        else:
            cue["start"] = round(first, 2)
            cue["end"] = round(last, 2)
            previous_end = cue["end"]
        cues.append(cue)
    return cues, exceptions, len(matched), len(source)


def process(job):
    payload = job["payload_snapshot"]
    with db() as connection:
        current = connection.execute("SELECT kind,payload FROM lesson_items WHERE id=%s", (job["item_id"],)).fetchone()
        if not current or current["kind"] != job["kind"]:
            raise ValueError("source_changed_since_queue")
    if job["operation"] == "translate":
        return process_translation(job, current["payload"])
    if job["operation"] != "synchronize" or current["payload"] != payload:
        raise ValueError("source_changed_since_queue")
    with tempfile.TemporaryDirectory(prefix="leksuu-job-") as folder:
        media, digest = fetch_original(payload.get("url", ""), folder)
        if payload.get("audio_sha256") and digest.lower() != payload["audio_sha256"].lower():
            raise ValueError("audio_checksum_mismatch")
        duration = audio_duration(media)
        transcript = payload.get("transcript") or []
        language = (payload.get("language") or "en").lower()
        if not re.fullmatch(r"[a-z]{2,3}(?:-[a-z]{2,4})?", language):
            raise ValueError("invalid_audio_language")
        offset = float(payload.get("media_offset_seconds") or 0)
        if not 0 <= offset <= duration:
            raise ValueError("invalid_media_offset")
        # Correctly timed SRT/WebVTT is retained and range-checked against the
        # actual file. It is still returned for Admin review, not auto-approved.
        if transcript and all(c.get("start") is not None and c.get("end") is not None for c in transcript):
            previous = 0
            for cue in transcript:
                if not cue.get("text") or not previous <= cue["start"] < cue["end"] <= duration + 0.3:
                    raise ValueError("invalid_existing_subtitle_timing")
                previous = cue["end"]
            return {"audio_sha256": digest, "cues": [{"text": c["text"], "start": c["start"], "end": c["end"]} for c in transcript],
                    "exceptions": [], "method": "validated_existing_cues", "language": language,
                    "duration_seconds": duration, "human_editorial_approved": False}
        if job["result"] and job["result"].get("phase") == "asr_checkpoint" and job["result"].get("audio_sha256") == digest:
            witness = job["result"]["witness"]
        else:
            witness = transcribe(media, language)
            with db() as connection:
                connection.execute("""UPDATE media_processing_jobs SET result=%s,progress=70,updated_at=now()
                    WHERE id=%s AND status='running'""", (Jsonb({"phase": "asr_checkpoint", "audio_sha256": digest,
                                                              "witness": witness}), job["id"]))
                event(connection, job["id"], "asr_checkpoint_saved")
        if not transcript:
            transcript = [{"text": segment["text"].strip()} for segment in witness["segments"] if segment["text"].strip()]
            method = "generated_transcript_and_aligned"
        else:
            method = "speech_aligned_existing_transcript"
        if not transcript:
            raise ValueError("no_spoken_transcript_detected")
        onset = opening_sound_onset(media)
        cues, exceptions, matched, total = align(transcript, witness, duration, offset, onset)
        return {"audio_sha256": digest, "cues": cues, "exceptions": exceptions,
                "method": method, "language": language, "duration_seconds": duration,
                "opening_sound_onset_seconds": onset,
                "matched_words": matched, "source_words": total,
                "human_editorial_approved": False}


def finish(job_id, worker_id, result=None, error=None):
    with db() as connection:
        row = connection.execute("SELECT cancel_requested,status FROM media_processing_jobs WHERE id=%s FOR UPDATE", (job_id,)).fetchone()
        if not row or row["status"] != "running":
            return "lost_claim"
        if row["cancel_requested"]:
            next_status, error = "cancelled", "cancel_requested"
        elif error:
            next_status = "failed"
        else:
            next_status = "review"
        connection.execute("""UPDATE media_processing_jobs SET status=%s,progress=%s,result=coalesce(%s,result),
            error_code=%s,lease_until=NULL,worker_id=NULL,updated_at=now()
            WHERE id=%s AND worker_id=%s""", (next_status, 100 if next_status == "review" else 0,
                                               Jsonb(result) if result else None, error, job_id, worker_id))
        event(connection, job_id, next_status, error=error)
        return next_status


def run_once(worker_id):
    job = claim(worker_id)
    if not job:
        return False
    stop = threading.Event()
    pulse = threading.Thread(target=heartbeat, args=(job["id"], worker_id, stop), daemon=True)
    pulse.start()
    try:
        result = process(job)
        state = finish(job["id"], worker_id, result=result)
    except Exception as exc:
        code = str(exc) if isinstance(exc, ValueError) else type(exc).__name__
        if not re.fullmatch(r"[a-z0-9_]+", code):
            code = "processing_failed"
        state = finish(job["id"], worker_id, error=code)
    finally:
        stop.set()
        pulse.join(timeout=2)
    print(json.dumps({"job_id": str(job["id"]), "status": state}), flush=True)
    return True


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--once", action="store_true")
    args = parser.parse_args()
    worker_id = str(uuid.uuid4())
    if args.once:
        run_once(worker_id)
        return
    while True:
        if not run_once(worker_id):
            time.sleep(2)


if __name__ == "__main__":
    main()
