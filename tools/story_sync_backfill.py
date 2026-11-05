"""List or queue Story recordings with missing timing. Dry-run is the default.

Uses MEDIA_JOBS_DATABASE_URL. Export a database backup before --apply. This tool
never changes lesson payloads; the worker checks its source snapshot again.
"""
import argparse
import hashlib
import json
import os
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(tempfile.gettempdir()) / "leksuu-align-pydeps"))
import psycopg
from psycopg.rows import dict_row


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    dsn = os.environ["MEDIA_JOBS_DATABASE_URL"]
    with psycopg.connect(dsn, row_factory=dict_row) as connection:
        connection.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ")
        connection.execute("SET TRANSACTION READ ONLY" if not args.apply else "SET TRANSACTION READ WRITE")
        db = connection.execute("SELECT current_database() AS name, inet_server_addr()::text AS host").fetchone()
        job_table = connection.execute("SELECT to_regclass('public.media_processing_jobs') IS NOT NULL AS present").fetchone()["present"]
        if args.apply and not job_table:
            raise RuntimeError("Apply media job migrations through 0060 before --apply")
        rows = connection.execute("""SELECT id::text AS id, payload::text AS raw, payload
            FROM lesson_items WHERE kind='story' AND payload->>'url' IS NOT NULL
            ORDER BY id FOR UPDATE""" if args.apply else """SELECT id::text AS id, payload::text AS raw, payload
            FROM lesson_items WHERE kind='story' AND payload->>'url' IS NOT NULL ORDER BY id""").fetchall()
        report = []
        for row in rows:
            payload = row["payload"]
            cues = payload.get("transcript") or []
            missing = sum(not isinstance(cue.get("start"), (int, float)) or
                          not isinstance(cue.get("end"), (int, float)) for cue in cues)
            if not cues or missing == 0 or payload.get("timing_status") in ("approved", "human_approved"):
                continue
            source = payload.get("alignment_source") or {}
            if source == {"url": payload.get("url", ""), "audio_sha256": payload.get("audio_sha256", ""),
                          "language": payload.get("language", "en"),
                          "transcript_texts": [cue.get("text") for cue in cues]}:
                continue
            digest = hashlib.sha256(row["raw"].encode()).hexdigest()
            existing = connection.execute("""SELECT id::text AS id,status,attempts FROM media_processing_jobs
                WHERE item_id=%s::uuid AND payload_sha256=%s AND operation='synchronize' AND status IN
                ('pending','running','review','ready','approved','failed') ORDER BY created_at DESC LIMIT 1""", (row["id"], digest)).fetchone() if job_table else None
            entry = {"id": row["id"], "title": payload.get("title", ""), "missing_paragraphs": missing,
                     "total_paragraphs": len(cues), "existing_job": existing}
            if existing:
                entry["action"] = "already_queued_or_review" if existing["status"] != "failed" else "inspect_failed_job"
            else:
                entry["action"] = "queue"
                if args.apply:
                    job = connection.execute("""INSERT INTO media_processing_jobs
                        (item_id,kind,operation,target_language,payload_sha256,payload_snapshot,events)
                        VALUES (%s::uuid,'story','synchronize','',%s,%s::jsonb,'[{"event":"queued_by_backfill"}]'::jsonb)
                        ON CONFLICT (item_id,operation,target_language,payload_sha256)
                        WHERE status IN ('pending','running','review','ready')
                        DO UPDATE SET updated_at=media_processing_jobs.updated_at RETURNING id::text""",
                        (row["id"], digest, row["raw"])).fetchone()
                    entry["job_id"] = job["id"]
            report.append(entry)
        print(json.dumps({"database": db, "job_table_present": job_table, "dry_run": not args.apply, "affected": report}, indent=2, default=str))


if __name__ == "__main__":
    main()
