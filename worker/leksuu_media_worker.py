"""Durable PostgreSQL media synchronization worker for Stories, Podcasts and Films.

Run separately from the API. Verified alignment fills missing timing only;
uncertain passages remain for review. One worker processes one job at a
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

from hls_pipeline import run_ffmpeg, upload_hls

sys.path.insert(0, str(Path(tempfile.gettempdir()) / "leksuu-align-pydeps"))
import av  # noqa: E402
import psycopg  # noqa: E402
from psycopg.rows import dict_row  # noqa: E402
from psycopg.types.json import Jsonb  # noqa: E402
from faster_whisper import WhisperModel  # noqa: E402
from faster_whisper.audio import decode_audio  # noqa: E402

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
ALIGNMENT_VERSION = 2


def words(text):
    return TOKEN.findall(text.casefold())


def exact_word_timing(cue):
    tokens = [token for token in cue.get("text", "").split() if TOKEN.search(token)]
    timed = cue.get("words")
    start, end = cue.get("start"), cue.get("end")
    if not tokens or not isinstance(start, (int, float)) or not isinstance(end, (int, float)) or \
            not isinstance(timed, list) or len(timed) != len(tokens):
        return False
    previous = start
    for word in timed:
        word_start, word_end = word.get("start"), word.get("end")
        if not isinstance(word_start, (int, float)) or not isinstance(word_end, (int, float)) or \
                word_start < previous or word_end <= word_start or word_start < start or word_end > end + 0.05:
            return False
        previous = word_end
    return True


def alignment_identity(payload):
    return {"url": payload.get("url", ""), "audio_sha256": payload.get("audio_sha256", ""),
            "language": payload.get("language", "en"),
            "transcript_texts": [cue.get("text", "") for cue in (payload.get("transcript") or [])]}


def needs_story_synchronization(payload):
    if not payload.get("url"):
        return False
    transcript = payload.get("transcript") or []
    if not transcript:
        return True
    complete = all(cue.get("timing_status") == "not_in_audio" or exact_word_timing(cue)
                   for cue in transcript)
    if complete:
        return False
    return not (payload.get("alignment_version", 0) >= ALIGNMENT_VERSION and
                payload.get("alignment_source") == alignment_identity(payload))


def reconcile_story_jobs():
    """Queue current story payloads once; API enqueue remains the fast path."""
    queued = 0
    with db() as connection:
        rows = connection.execute("""SELECT id,payload FROM lesson_items
            WHERE kind='story' AND nullif(payload->>'url','') IS NOT NULL ORDER BY id""").fetchall()
        for row in rows:
            payload = row["payload"]
            if not needs_story_synchronization(payload):
                continue
            existing = connection.execute("""SELECT 1 FROM media_processing_jobs
                WHERE item_id=%s AND operation='synchronize' AND target_language=''
                  AND payload_snapshot=%s AND status IN ('pending','running','review','ready','approved','failed')
                LIMIT 1""", (row["id"], Jsonb(payload))).fetchone()
            if existing:
                continue
            raw = json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
            digest = hashlib.sha256(raw.encode()).hexdigest()
            connection.execute("""INSERT INTO media_processing_jobs
                (item_id,kind,operation,target_language,payload_sha256,payload_snapshot,events)
                VALUES (%s,'story','synchronize','',%s,%s,'[{"event":"queued_automatically"}]'::jsonb)
                ON CONFLICT (item_id,operation,target_language,payload_sha256)
                WHERE status IN ('pending','running','review','ready') DO NOTHING""",
                (row["id"], digest, Jsonb(payload)))
            queued += 1
    if queued:
        print(json.dumps({"event": "story_reconciliation", "queued": queued}), flush=True)
    return queued


def db():
    database_url = os.environ.get("MEDIA_JOBS_DATABASE_URL") or os.environ["DATABASE_URL"]
    return psycopg.connect(database_url, row_factory=dict_row)


def event(connection, job_id, name, **details):
    connection.execute("UPDATE media_processing_jobs SET events=events||%s::jsonb,updated_at=now() WHERE id=%s",
                       (Jsonb([{"event": name, **details}]), job_id))


def claim(worker_id):
    with db() as connection:
        connection.execute("UPDATE media_processing_jobs SET status='failed',worker_id=NULL,lease_until=NULL,"
                           "error_code='worker_interrupted_retry_exhausted',updated_at=now(),"
                           "events=events||'[{\"event\":\"worker_interrupted_retry_exhausted\"}]'::jsonb "
                           "WHERE status='running' AND lease_until<now() AND attempts>=3")
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
    key = path.removeprefix("/media/")
    media = Path(folder) / ("original" + (Path(path).suffix or ".bin"))
    maximum = int(os.environ.get("MEDIA_JOBS_MAX_BYTES", str(2 << 30)))
    if maximum <= 0:
        raise ValueError("invalid_media_size_limit")
    digest = hashlib.sha256()
    size = 0
    if os.environ.get("S3_BUCKET"):
        import boto3
        endpoint = os.environ.get("S3_ENDPOINT", "")
        if endpoint and not endpoint.startswith(("https://", "http://")):
            endpoint = ("https://" if os.environ.get("S3_SECURE", "true") == "true" else "http://") + endpoint
        client = boto3.client("s3", endpoint_url=endpoint or None,
                              region_name=os.environ.get("S3_REGION", "us-east-1"),
                              aws_access_key_id=os.environ["S3_ACCESS_KEY"],
                              aws_secret_access_key=os.environ["S3_SECRET_KEY"])
        stored_key = "/".join(part for part in (os.environ.get("S3_PREFIX", "").strip("/"), key) if part)
        source = client.get_object(Bucket=os.environ["S3_BUCKET"], Key=stored_key)["Body"]
    elif os.environ.get("MEDIA_JOBS_LOCAL_MEDIA_DIR"):
        source = (Path(os.environ["MEDIA_JOBS_LOCAL_MEDIA_DIR"]) / key).open("rb")
    else:
        request = urllib.request.Request(base + path, headers={"User-Agent": "LeksuuMediaWorker/1"})
        source = urllib.request.urlopen(request, timeout=60)
    with source, media.open("wb") as destination:
        while chunk := source.read(1 << 20):
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
    model_name = os.environ.get("MEDIA_JOBS_WHISPER_MODEL") or ("small.en" if language == "en" else "small")
    if MODEL_NAME != model_name:
        model_root = os.environ.get("MEDIA_JOBS_MODEL_DIR",
                                str(Path(tempfile.gettempdir()) / "leksuu-whisper-models"))
        options = {"device": "cpu", "compute_type": "int8",
                   "cpu_threads": max(1, int(os.environ.get("MEDIA_JOBS_CPU_THREADS", "2"))),
                   "download_root": model_root}
        try:
            MODEL = WhisperModel(model_name, local_files_only=True, **options)
        except (OSError, RuntimeError, ValueError):
            MODEL = WhisperModel(model_name, local_files_only=False, **options)
        MODEL_NAME = model_name
    whisper_lang = {"ckb": "ku"}.get(language, language)
    # Decode once, then cover every minute with overlapping context. Speech VAD
    # can discard singing; never use it to decide which parts deserve ASR.
    audio = decode_audio(str(path), sampling_rate=16000)
    duration = len(audio) / 16000
    result = []
    for core in range(0, len(audio), 60 * 16000):
        left = max(0, core - 2 * 16000)
        right = min(len(audio), core + 62 * 16000)
        segments, _ = MODEL.transcribe(audio[left:right], language=whisper_lang, beam_size=5,
                                      word_timestamps=True, vad_filter=False,
                                      condition_on_previous_text=False)
        for part in segments:
            kept = []
            for word in part.words or []:
                start, end = word.start + left / 16000, word.end + left / 16000
                midpoint = (start + end) / 2
                if core / 16000 <= midpoint < min(duration, core / 16000 + 60) and end > start:
                    kept.append({"start": max(0, start), "end": min(duration, end),
                                 "word": word.word, "probability": word.probability})
            # Split long model segments into readable lines using observed words.
            # No interpolation across instrumental breaks or missing lyrics.
            group = []
            for word in kept:
                if group and (len(group) >= 12 or word["start"] - group[-1]["end"] > 2 or
                              word["end"] - group[0]["start"] > 8):
                    result.append(asr_line(group))
                    group = []
                group.append(word)
            if group:
                result.append(asr_line(group))
    return {"language": language, "duration_seconds": duration, "segments": result,
            "transcription_version": 2, "model": model_name}


def asr_line(group):
    return {"start": group[0]["start"], "end": group[-1]["end"],
            "text": "".join(word["word"] for word in group).strip(), "words": group}



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
            window_start = max(0, pairs[0][1] - 4) if pairs else None
            window_end = min(len(spoken), pairs[-1][1] + 5) if pairs else None
            exceptions.append({"paragraph": i + 1, "reasons": reasons,
                               "matched_words": len(pairs), "source_words": count,
                               "match_ratio": round(len(pairs) / count, 3) if count else 0,
                               "candidate_start": round(first, 2) if first is not None else None,
                               "candidate_end": round(last, 2) if last is not None else None,
                               "asr_context": " ".join(word[0] for word in spoken[window_start:window_end]) if window_start is not None else ""})
        else:
            cue["start"] = round(first, 2)
            cue["end"] = round(last, 2)
            # Each timestamp comes from a matched ASR word in this recording.
            # A partial match can locate a paragraph but cannot time every word.
            if len(pairs) == count and count and len([token for token in paragraph["text"].split() if TOKEN.search(token)]) == count:
                exact = [spoken[matched[j]] for j in positions]
                if all(exact[k][1] < exact[k][2] and
                       (k == 0 or exact[k - 1][2] <= exact[k][1])
                       for k in range(len(exact))):
                    cue["words"] = [{"word": source[j][0], "start": round(exact[k][1], 3),
                                     "end": round(exact[k][2], 3)} for k, j in enumerate(positions)]
            previous_end = cue["end"]
        cues.append(cue)
    matched_spoken = set(matched.values())
    extra = []
    run = []
    for j in range(len(spoken) + 1):
        if j < len(spoken) and j not in matched_spoken:
            run.append(j)
        elif run:
            if len(run) >= 6:
                extra.append({"start": round(spoken[run[0]][1], 2), "end": round(spoken[run[-1]][2], 2),
                              "words": len(run), "text": " ".join(spoken[k][0] for k in run)})
            run = []
    return cues, exceptions, extra, len(matched), len(source)


def process(job):
    payload = job["payload_snapshot"]
    with db() as connection:
        current = connection.execute("SELECT kind,payload FROM lesson_items WHERE id=%s", (job["item_id"],)).fetchone()
        if not current or current["kind"] != job["kind"]:
            raise ValueError("source_changed_since_queue")
    if job["operation"] == "translate":
        return process_translation(job, current["payload"])
    if job["operation"] == "transcode":
        if current["payload"] != payload or job["kind"] not in ("video", "music"):
            raise ValueError("source_changed_since_queue")
        with tempfile.TemporaryDirectory(prefix="leksuu-hls-") as folder:
            media, digest = fetch_original(payload.get("url", ""), folder)
            expected = payload.get("audio_sha256") or payload.get("media_sha256")
            if expected and digest.lower() != expected.lower():
                raise ValueError("media_checksum_mismatch")
            output = Path(folder) / "hls"
            details = run_ffmpeg(media, output)
            prefix = "hls/" + str(job["item_id"]) + "/" + job["payload_sha256"][:16]
            manifest = upload_hls(output, prefix)
            return {"hls_manifest_key": manifest, "source_sha256": digest,
                    "profiles": details["profiles"], "duration_seconds": details["duration_seconds"]}
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
        # This worker transcribes the complete MP3; its ASR timestamps already
        # include any spoken introduction. An additional offset would double it.
        offset = float(payload.get("media_offset_seconds") or 0)
        if offset != 0:
            raise ValueError("media_offset_not_supported_for_full_recording")
        all_existing_timed = bool(transcript) and all(c.get("start") is not None and c.get("end") is not None for c in transcript)
        if all_existing_timed:
            previous = 0
            for cue in transcript:
                if not cue.get("text") or not previous <= cue["start"] < cue["end"] <= duration + 0.3:
                    raise ValueError("invalid_existing_subtitle_timing")
                previous = cue["end"]
        if job["result"] and job["result"].get("phase") == "asr_checkpoint" and job["result"].get("audio_sha256") == digest and job["result"].get("witness", {}).get("transcription_version") == 2:
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
        cues, exceptions, extra, matched, total = align(transcript, witness, duration, offset, onset)
        if job["kind"] == "music":
            boundaries = [(segment["start"], segment["end"]) for segment in witness["segments"]]
            previous = 0.0
            for start, end in boundaries + [(duration, duration)]:
                if start - previous > 20:
                    exceptions.append({"reasons": ["possible_missing_lyrics_or_instrumental"],
                                       "candidate_start": previous, "candidate_end": start})
                previous = max(previous, end)
            for i, cue in enumerate(cues):
                if cue.get("end", 0) - cue.get("start", 0) > 12:
                    exceptions.append({"paragraph": i + 1, "reasons": ["suspicious_long_lyric_timing"]})
        if all_existing_timed:
            for i, cue in enumerate(cues):
                original = transcript[i]
                disagrees = cue.get("start") is None or abs(cue["start"] - original["start"]) > 1 or \
                    abs(cue["end"] - original["end"]) > 1
                if disagrees:
                    exceptions.append({"paragraph": i + 1, "reasons": ["existing_timing_disagrees_with_asr"]})
                    cue.pop("words", None)
                elif cue.get("words") and not all(original["start"] <= word["start"] < word["end"] <= original["end"] + 0.05
                                                   for word in cue["words"]):
                    cue.pop("words", None)
                cue["start"] = original["start"]
                cue["end"] = original["end"]
                cue.pop("timing_status", None)
        return {"audio_sha256": digest, "cues": cues, "exceptions": exceptions,
                "unmatched_spoken_runs": extra,
                "method": "validated_existing_cues_with_asr" if all_existing_timed else method,
                "language": language, "duration_seconds": duration,
                "opening_sound_onset_seconds": onset,
                "matched_words": matched, "source_words": total,
                "human_editorial_approved": False}


def merge_synchronization(source, result, job_id):
    cues = source.get("transcript") or []
    aligned = result.get("cues") or []
    generated = not cues
    if not aligned or (cues and len(cues) != len(aligned)):
        raise ValueError("transcript_changed_since_queue")
    updated = json.loads(json.dumps(source))
    if generated:
        updated["transcript"] = [{"text": candidate["text"]} for candidate in aligned]
        cues = updated["transcript"]
        updated["transcript_status"] = "needs_review"
    unresolved = generated or bool(result.get("exceptions") or result.get("unmatched_spoken_runs"))
    for original, candidate in zip(updated["transcript"], aligned):
        if original.get("text") != candidate.get("text"):
            raise ValueError("transcript_changed_since_queue")
        if original.get("timing_status") == "not_in_audio" or original.get("review_decision") in ("accepted", "corrected", "not_in_audio"):
            continue
        if original.get("start") is not None or original.get("end") is not None:
            # Paragraph timing is immutable; exact ASR word timing may enrich
            # it when process() verified the candidate fits its boundaries.
            if candidate.get("words"):
                original["words"] = candidate["words"]
            continue
        if candidate.get("start") is None or candidate.get("end") is None:
            unresolved = True
            original["timing_status"] = "unverified"
            continue
        original["start"] = candidate["start"]
        original["end"] = candidate["end"]
        if candidate.get("words"):
            original["words"] = candidate["words"]
        original.pop("timing_status", None)
    updated["timing_status"] = "needs_review" if unresolved else "ready"
    updated["alignment_job_id"] = str(job_id)
    updated["alignment_source"] = alignment_identity(source)
    updated["alignment_version"] = ALIGNMENT_VERSION
    return updated, unresolved


def finish(job_id, worker_id, result=None, error=None):
    with db() as connection:
        row = connection.execute("SELECT cancel_requested,status,item_id,payload_snapshot,operation FROM media_processing_jobs WHERE id=%s FOR UPDATE", (job_id,)).fetchone()
        if not row or row["status"] != "running":
            return "lost_claim"
        if row["cancel_requested"]:
            next_status, error = "cancelled", "cancel_requested"
        elif error:
            next_status = "failed"
        else:
            next_status = "review"
            if row["operation"] == "synchronize":
                try:
                    source = row["payload_snapshot"]
                    current = connection.execute("SELECT payload FROM lesson_items WHERE id=%s FOR UPDATE", (row["item_id"],)).fetchone()
                    if not current or current["payload"] != source:
                        raise ValueError("source_changed_since_queue")
                    updated, unresolved = merge_synchronization(source, result, job_id)
                    connection.execute("UPDATE lesson_items SET payload=%s WHERE id=%s AND payload=%s",
                                       (Jsonb(updated), row["item_id"], Jsonb(source)))
                    next_status = "review" if unresolved else "ready"
                except ValueError as exc:
                    next_status, error = "failed", str(exc)
            elif row["operation"] == "transcode":
                source = row["payload_snapshot"]
                current = connection.execute("SELECT payload FROM lesson_items WHERE id=%s FOR UPDATE", (row["item_id"],)).fetchone()
                if not current or current["payload"] != source:
                    next_status, error = "failed", "source_changed_since_queue"
                else:
                    updated = dict(source)
                    updated["hls_manifest_key"] = result["hls_manifest_key"]
                    updated["hls_status"] = "ready"
                    connection.execute("UPDATE lesson_items SET payload=%s WHERE id=%s AND payload=%s",
                                       (Jsonb(updated), row["item_id"], Jsonb(source)))
                    next_status = "ready"
        connection.execute("""UPDATE media_processing_jobs SET status=%s,progress=%s,result=coalesce(%s,result),
            error_code=%s,lease_until=NULL,worker_id=NULL,updated_at=now()
            WHERE id=%s AND worker_id=%s""", (next_status, 100 if next_status in ("review", "ready") else 0,
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
        from job_errors import failure_code
        code = failure_code(exc, job["operation"])
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
    reconcile_story_jobs()
    interval = max(30, int(os.environ.get("MEDIA_JOBS_RECONCILE_SECONDS", "300")))
    next_reconcile = time.monotonic() + interval
    while True:
        if not run_once(worker_id):
            time.sleep(2)
        if time.monotonic() >= next_reconcile:
            try:
                reconcile_story_jobs()
            except Exception as exc:
                print(json.dumps({"event": "story_reconciliation_failed", "error": type(exc).__name__}), flush=True)
            next_reconcile = time.monotonic() + interval


if __name__ == "__main__":
    main()
