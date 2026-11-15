"""Worker orchestration tests with inference and PostgreSQL replaced by fixtures.

Load the actual function bodies without importing optional native dependencies.
These test state transitions, not the quality of real model output.
"""
import ast
import copy
import hashlib
import json
import os
import unittest
from pathlib import Path
from unittest.mock import patch

import media_automation


def load_functions(database):
    source = Path(__file__).with_name("leksuu_media_worker.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    names = {"automation_enabled", "reconcile_media_jobs", "merge_synchronization", "alignment_identity", "translation_source", "same_alignment_input", "finish"}
    functions = [node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name in names]
    scope = {"os": os, "json": json, "hashlib": hashlib, "ALIGNMENT_VERSION": 2,
             "Jsonb": lambda value: value, "db": lambda: database, "event": lambda *args, **kwargs: None}
    scope.update({name: getattr(media_automation, name) for name in (
        "next_stage", "accept_source", "apply_translations", "translation_hash", "media_source", "source_input")})
    exec(compile(ast.Module(body=functions, type_ignores=[]), "worker", "exec"), scope)
    return scope


class Rows:
    def __init__(self, rows): self.rows = rows
    def fetchone(self): return self.rows[0] if self.rows else None
    def fetchall(self): return self.rows


class Database:
    def __init__(self, kind="music"):
        self.item = {"id": "item", "kind": kind, "payload": {"published": False, "url": "/media/source.mp3", "language": "en", "transcript": None}}
        self.jobs = []

    def __enter__(self): return self
    def __exit__(self, *_): return False

    def execute(self, sql, arguments=()):
        if sql.startswith("SELECT id,kind,payload"):
            return Rows([{**copy.deepcopy(self.item), "payload_text": json.dumps(self.item["payload"])}])
        if sql.startswith("SELECT 1 FROM media_processing_jobs"):
            if len(arguments) == 1:
                return Rows([job for job in self.jobs if job["status"] in ("pending", "running")])
            _, operation, target, payload = arguments
            return Rows([job for job in self.jobs if job["operation"] == operation and job["target_language"] == target and job["payload_snapshot"] == payload])
        if sql.startswith("INSERT INTO media_processing_jobs"):
            item_id, kind, operation, target, digest, payload = arguments
            self.jobs.append({"id": str(len(self.jobs)), "item_id": item_id, "kind": kind,
                              "operation": operation, "target_language": target, "payload_sha256": digest,
                              "payload_snapshot": copy.deepcopy(payload), "status": "pending", "cancel_requested": False,
                              "worker_id": "worker"})
            return Rows([])
        if sql.startswith("SELECT cancel_requested"):
            return Rows([job for job in self.jobs if job["id"] == arguments[0]])
        if sql.startswith("SELECT payload FROM lesson_items"):
            return Rows([{"payload": copy.deepcopy(self.item["payload"])}])
        if sql.startswith("UPDATE lesson_items SET payload"):
            replacement, _, expected = arguments
            if self.item["payload"] != expected:
                raise AssertionError("payload compare-and-swap bypassed")
            self.item["payload"] = copy.deepcopy(replacement)
            return Rows([])
        if sql.startswith("UPDATE media_processing_jobs SET status"):
            status, _, result, error, _delay, _delay_again, job_id, _ = arguments
            job = next(job for job in self.jobs if job["id"] == job_id)
            job.update(status=status, result=result, error_code=error)
            return Rows([])
        raise AssertionError(sql)


class AutomaticWorkerTest(unittest.TestCase):
    def setUp(self):
        self.environment = patch.dict(os.environ, {"MEDIA_JOBS_AUTOMATION_ENABLED": "true"})
        self.environment.start()
        self.addCleanup(self.environment.stop)

    def test_upload_to_prepared_without_manual_stage_actions_for_all_types(self):
        for kind in ("music", "video", "podcast", "story"):
            database = Database(kind)
            functions = load_functions(database)
            for _ in range(5):
                if not functions["reconcile_media_jobs"]():
                    break
                job = database.jobs[-1]
                job["status"] = "running"
                if job["operation"] == "transcode":
                    result = {"hls_manifest_key": "hls/item/version/master.m3u8"}
                elif job["operation"] == "synchronize":
                    result = {"cues": [{"text": "Hello there", "start": 1, "end": 2}],
                              "automatic_source_check": {"passed": True, "issues": [], "verification_model": "independent"}}
                else:
                    result = {"target_language": job["target_language"], "source_sha256": job["payload_sha256"],
                              "translations": [{"paragraph": 1, "text": "Hello there",
                                                "translation": {"fa": "سلام", "ckb": "سڵاو"}[job["target_language"]],
                                                "automatic_check": {"passed": True, "independent_model": True,
                                                                    "calibration": {"passed": True}}}]}
                self.assertEqual(functions["finish"](job["id"], "worker", result=result), "ready")
            self.assertIsNone(media_automation.next_stage(kind, database.item["payload"]))
            self.assertFalse(database.item["payload"]["published"])
            self.assertNotIn("rights_approved", database.item["payload"])
            self.assertEqual(database.item["payload"]["timing_status"], "ready")
            self.assertEqual(database.item["payload"]["transcript"][0]["translation_review"], {"fa": "auto_approved", "ckb": "auto_approved"})
            self.assertEqual(functions["reconcile_media_jobs"](), 0)

    def test_active_and_failed_jobs_do_not_requeue(self):
        database = Database()
        functions = load_functions(database)
        self.assertEqual(functions["reconcile_media_jobs"](), 1)
        self.assertEqual(functions["reconcile_media_jobs"](), 0)
        database.jobs[0]["status"] = "failed"
        self.assertEqual(functions["reconcile_media_jobs"](), 0)
        self.assertEqual(len(database.jobs), 1)

    def test_cancelled_or_lost_claim_cannot_change_the_draft(self):
        database = Database()
        functions = load_functions(database)
        functions["reconcile_media_jobs"]()
        job = database.jobs[0]
        job["status"] = "running"
        original = copy.deepcopy(database.item)
        result = {"hls_manifest_key": "hls/item/version/master.m3u8"}
        self.assertEqual(functions["finish"](job["id"], "stale-worker", result=result), "lost_claim")
        job["cancel_requested"] = True
        self.assertEqual(functions["finish"](job["id"], "worker", result=result), "cancelled")
        self.assertEqual(original, database.item)

    def test_transient_provider_failure_schedules_bounded_retry(self):
        database = Database("podcast")
        functions = load_functions(database)
        functions["reconcile_media_jobs"]()
        job = database.jobs[0]
        job.update(status="running", attempts=1)
        self.assertEqual(functions["finish"](job["id"], "worker", error="provider_timeout"), "pending")
        self.assertEqual(job["error_code"], "provider_timeout")
        job.update(status="running", attempts=3)
        self.assertEqual(functions["finish"](job["id"], "worker", error="provider_timeout"), "failed")

    def test_source_edit_while_processing_is_not_overwritten(self):
        database = Database()
        functions = load_functions(database)
        functions["reconcile_media_jobs"]()
        job = database.jobs[0]
        job["status"] = "running"
        database.item["payload"]["url"] = "/media/replacement.mp3"
        result = {"hls_manifest_key": "hls/item/version/master.m3u8"}
        self.assertEqual(functions["finish"](job["id"], "worker", result=result), "failed")
        self.assertEqual(job["error_code"], "source_changed_since_queue")
        self.assertNotIn("hls_manifest_key", database.item["payload"])

    def test_hls_keeps_unrelated_metadata_edited_after_queue(self):
        database = Database()
        functions = load_functions(database)
        functions["reconcile_media_jobs"]()
        job = database.jobs[0]
        job["status"] = "running"
        database.item["payload"].update(title="Corrected title", cover="/media/cover.jpg")
        result = {"hls_manifest_key": "hls/item/version/master.m3u8"}
        self.assertEqual(functions["finish"](job["id"], "worker", result=result), "ready")
        self.assertEqual(database.item["payload"]["title"], "Corrected title")
        self.assertEqual(database.item["payload"]["cover"], "/media/cover.jpg")
        self.assertEqual(database.item["payload"]["hls_manifest_key"], result["hls_manifest_key"])

    def test_alignment_keeps_unrelated_metadata_edited_after_queue(self):
        database = Database("podcast")
        functions = load_functions(database)
        functions["reconcile_media_jobs"]()
        job = database.jobs[0]
        job["status"] = "running"
        database.item["payload"].update(title="Corrected title", rights_reference="fixture-license")
        result = {"cues": [{"text": "Hello there", "start": 1, "end": 2}],
                  "automatic_source_check": {"passed": True, "issues": [], "verification_model": "independent"}}
        self.assertEqual(functions["finish"](job["id"], "worker", result=result), "ready")
        self.assertEqual(database.item["payload"]["title"], "Corrected title")
        self.assertEqual(database.item["payload"]["rights_reference"], "fixture-license")
        self.assertEqual(database.item["payload"]["transcript"][0]["start"], 1)

    def test_alignment_input_ignores_metadata_but_detects_source_and_timing(self):
        functions = load_functions(Database())
        original = {"published": False, "url": "/media/source.mp3", "language": "en",
                    "transcript": [{"text": "Hello", "start": 1, "end": 2}]}
        edited = copy.deepcopy(original)
        edited["title"] = "New title"
        self.assertTrue(functions["same_alignment_input"](edited, original))
        edited["transcript"][0]["start"] = 1.5
        self.assertFalse(functions["same_alignment_input"](edited, original))
        edited = copy.deepcopy(original)
        edited["url"] = "/media/replacement.mp3"
        self.assertFalse(functions["same_alignment_input"](edited, original))
        edited = copy.deepcopy(original)
        edited["transcript"][0]["timing_status"] = "not_in_audio"
        self.assertFalse(functions["same_alignment_input"](edited, original))

    def test_automation_can_be_disabled(self):
        with patch.dict(os.environ, {"MEDIA_JOBS_AUTOMATION_ENABLED": "false"}):
            self.assertEqual(load_functions(Database())["reconcile_media_jobs"](), 0)


if __name__ == "__main__":
    unittest.main()
