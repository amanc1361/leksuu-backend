"""Reference gating and targeted draft correction; no model output is a reference."""
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from media_automation import apply_translations, automatic_publication_check
from translation_quality import candidate_risks, canonicalize_persian, evaluate_reference_file
from translation_providers import cloud_job_budget, translate_cloud
from leksuu_media_worker import process_translation


class TranslationQualityTest(unittest.TestCase):
    def test_targeted_machine_retry_keeps_cue_identity_and_timing(self):
        from contextlib import nullcontext
        class Connection:
            def execute(self, *_args): return None
        cue = {"cue_id": "cue-9", "text": "That plan is a piece of cake.",
               "start": 2.25, "end": 4.75}
        current = {"published": False, "language": "en", "transcript": [cue]}
        job = {"id": "job-9", "payload_snapshot": current, "payload_sha256": "snapshot-9",
               "target_language": "fa", "result": {}}
        with patch("leksuu_media_worker.db", return_value=nullcontext(Connection())), \
             patch("leksuu_media_worker.event"), \
             patch("leksuu_media_worker.automation_enabled", return_value=False), \
             patch("leksuu_media_worker.translate_text", side_effect=[
                 "این نقشه یک تکه کیک است.", "این برنامه خیلی آسان است."]) as translator:
            result = process_translation(job, current)
        self.assertEqual(translator.call_count, 2)
        self.assertEqual(result["translations"][0]["attempts"], 2)
        self.assertEqual(result["translations"][0]["cue_id"], "cue-9")
        self.assertEqual(result["translations"][0]["translation"], "این برنامه خیلی آسان است.")
        self.assertEqual((cue["cue_id"], cue["start"], cue["end"]), ("cue-9", 2.25, 4.75))
        self.assertFalse(result["translations"][0]["automatic_check"]["passed"])

    def test_cloud_providers_are_disabled_and_use_explicit_sorani_codes(self):
        with patch.dict("os.environ", {"MEDIA_JOBS_TRANSLATION_PROVIDER": "google",
                                   "MEDIA_JOBS_CLOUD_TRANSLATION_ALLOWED": "false"}):
            with self.assertRaisesRegex(ValueError, "not_authorized"):
                cloud_job_budget(10)
        class Response:
            def __init__(self, data): self.data = data
            def __enter__(self): return self
            def __exit__(self, *_args): return False
            def read(self, *_args): return self.data
        requests = []
        def google(request, timeout):
            requests.append((request, timeout))
            return Response(b'{"data":{"translations":[{"translatedText":"\\u0633\\u0695\\u0627\\u0648"}]}}')
        with patch.dict("os.environ", {"MEDIA_JOBS_TRANSLATION_PROVIDER": "google",
                                   "MEDIA_JOBS_CLOUD_TRANSLATION_ALLOWED": "true",
                                   "MEDIA_JOBS_CLOUD_MAX_CHARS_PER_JOB": "20",
                                   "LEKSUU_GOOGLE_TRANSLATE_API_KEY": "test-only",
                                   "MEDIA_JOBS_CLOUD_MIN_INTERVAL_SECONDS": "0"}):
            self.assertEqual(translate_cloud("Hello", "ckb", opener=google), "سڕاو")
            self.assertEqual(json.loads(requests[-1][0].data)["target"], "ckb")
            self.assertNotIn("test-only", requests[-1][0].full_url)
            with self.assertRaisesRegex(ValueError, "budget_exceeded"):
                cloud_job_budget(21)
        def azure(request, timeout):
            requests.append((request, timeout))
            return Response(b'[{"translations":[{"text":"\\u0633\\u0695\\u0627\\u0648"}]}]')
        with patch.dict("os.environ", {"MEDIA_JOBS_TRANSLATION_PROVIDER": "azure",
                                   "MEDIA_JOBS_CLOUD_TRANSLATION_ALLOWED": "true",
                                   "MEDIA_JOBS_CLOUD_MAX_CHARS_PER_JOB": "20",
                                   "LEKSUU_AZURE_TRANSLATOR_KEY": "test-only",
                                   "MEDIA_JOBS_CLOUD_MIN_INTERVAL_SECONDS": "0"}):
            translate_cloud("Hello", "ckb", opener=azure)
            self.assertIn("to=ku", requests[-1][0].full_url)
            self.assertNotIn("test-only", requests[-1][0].full_url)

    def test_phase2c_real_failures_are_flagged_without_approving_a_candidate(self):
        self.assertIn("literal_idiom", candidate_risks(
            "That plan is a piece of cake.", "این نقشه یک تکه کیک است.", "fa"))
        self.assertIn("repeated_proposition", candidate_risks(
            "Sara met Dilan in Erbil on Tuesday.",
            "سارا ڕۆژی سێشەممە دیلانی بینی لە ئەربیل، رۆژی سێشەممە سارا دیلانی لە ئەربیل بینی.", "ckb"))
        self.assertIn("repeated_proposition", candidate_risks(
            "Sara met Dilan in Erbil on Tuesday.",
            "سارا ڕۆژی سێشەممە دیلانی بینی لە ئەربیل، رۆژی سێ شەممە دیلان و سارا یەکتریان بینی.", "ckb"))
        self.assertIn("possible_added_clause", candidate_risks(
            "Wait here. I'll be right back.",
            "لێرە بمێنەوە، من بە خێرایی دەگەڕێمەوە، لەوێ دەمێنمەوە.", "ckb"))
        self.assertIn("possible_plurality_loss", candidate_risks(
            "We have 4-month-old mice that are no longer diabetic.",
            "ئێستا مشکێکی تەمەن چوار مانگمان هەیە کە نەخۆشی شەکرەی نییە.", "ckb",
            "We have a 4-month-old mouse that is no longer diabetic."))
        self.assertIn("proper_noun_mismatch", candidate_risks(
            "Sara met Dilan in Erbil on Tuesday.", "سارا دیلان را در اربیل دید.", "fa",
            "Sara met Dilan in Baghdad on Tuesday."))
        self.assertIn("persian_orthography", candidate_risks(
            "Wait here.", "همينجا صبر کن.", "fa"))
        self.assertEqual(candidate_risks("Wait here.", canonicalize_persian("همينجا صبر کن."), "fa"), [])
        # A comma around an appositive in the independent reference is not an invented clause.
        self.assertNotIn("possible_added_clause", candidate_risks(
            "Ring also settled a lawsuit with competing security company, the ADT Corporation.",
            "رینگ همچنین یک دعوای قانونی را با شرکت امنیتی رقیب، شرکت ADT، حل و فصل کرد.", "fa"))

    def test_known_bad_translation_cannot_use_forged_positive_check(self):
        cue = {"cue_id": "cue-1", "text": "That plan is a piece of cake.", "start": 1.25, "end": 3.5}
        row = {"paragraph": 1, "cue_id": "cue-1", "text": cue["text"],
               "translation": "این نقشه یک تکه کیک است.",
               "automatic_check": {"passed": True, "independent_model": True,
                                   "calibration": {"passed": True}}}
        updated = apply_translations({"published": False, "transcript": [cue]},
                                     {"translations": [row]}, "fa", "job")
        self.assertEqual(updated["translation_status"]["fa"], "draft")
        self.assertEqual(updated["automation"]["translations"]["fa"]["issues"][0]["reasons"],
                         ["literal_idiom"])
        self.assertEqual([(c["cue_id"], c["start"], c["end"]) for c in updated["transcript"]],
                         [("cue-1", 1.25, 3.5)])

    def test_missing_and_self_referential_reference_sets_fail_closed(self):
        self.assertFalse(evaluate_reference_file("", "fa", lambda *_: "")['passed'])
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "references.json"
            path.write_text(json.dumps({"version": 1, "provenance": "generated_by_model",
                                        "cases": [{"source_language": "en", "source": str(i), "fa": "سلام"}
                                                  for i in range(12)]}), encoding="utf-8")
            self.assertFalse(evaluate_reference_file(path, "fa", lambda *_: "سلام")['passed'])

    def test_independent_reference_fixture_measures_each_language(self):
        cases = [{"source_language": "en", "source": "Source sentence " + str(i),
                  "fa": "فارسی " + str(i), "ckb": "سۆرانی " + str(i)} for i in range(12)]
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / "references.json"
            path.write_text(json.dumps({"version": 1, "provenance": "independent_editorial_reference",
                                        "cases": cases}, ensure_ascii=False), encoding="utf-8")
            self.assertTrue(evaluate_reference_file(path, "fa", lambda source, _: cases[int(source.split()[-1])]["fa"])["passed"])
            self.assertFalse(evaluate_reference_file(path, "ckb", lambda *_: "فارسی اشتباه")["passed"])

    def test_only_machine_draft_may_be_replaced(self):
        cue = {"cue_id": "cue-1", "text": "Hello", "translation": {"fa": "old"},
               "translation_review": {"fa": "draft"}, "translation_origin": {"fa": "machine"}}
        row = {"paragraph": 1, "cue_id": "cue-1", "text": "Hello", "translation": "new",
               "generated_by_worker": True, "automatic_check": {"passed": False}}
        updated = apply_translations({"published": False, "transcript": [cue]}, {"translations": [row]}, "fa", "job")
        self.assertEqual(updated["transcript"][0]["translation"]["fa"], "new")
        cue["translation_origin"]["fa"] = "human"
        with self.assertRaisesRegex(ValueError, "overwrite"):
            apply_translations({"published": False, "transcript": [cue]}, {"translations": [row]}, "fa", "job")
        cue["translation_origin"]["fa"] = "machine"
        cue["translation_review"]["fa"] = "approved"
        with self.assertRaisesRegex(ValueError, "overwrite"):
            apply_translations({"published": False, "transcript": [cue]}, {"translations": [row]}, "fa", "job")

    def test_uncalibrated_result_remains_draft(self):
        row = {"paragraph": 1, "text": "Hello", "translation": "سلام",
               "automatic_check": {"passed": True, "independent_model": True}}
        updated = apply_translations({"published": False, "transcript": [{"text": "Hello"}]},
                                     {"translations": [row]}, "fa", "job")
        self.assertEqual(updated["translation_status"]["fa"], "draft")

    def test_identical_persian_and_sorani_cannot_be_machine_approved(self):
        text = "\u0633\u0644\u0627\u0645"
        row = {"paragraph": 1, "cue_id": "cue-1", "text": "Hello", "translation": text,
               "automatic_check": {"passed": True, "independent_model": True,
                                   "calibration": {"passed": True}}}
        payload = {"published": False, "transcript": [{"cue_id": "cue-1", "text": "Hello",
                    "translation": {"fa": text}}]}
        updated = apply_translations(payload, {"translations": [row]}, "ckb", "job")
        self.assertEqual(updated["transcript"][0]["translation_review"]["ckb"], "draft")
        self.assertIn("language_substitution_suspected",
                      automatic_publication_check("podcast", updated))


if __name__ == "__main__":
    unittest.main()
