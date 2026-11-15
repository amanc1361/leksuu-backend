import copy
import unittest

from media_automation import (accept_source, apply_translations, identity, next_stage, translation_input, media_source, source_input,
                              source_assessment, translation_assessment, translation_hash, automatic_publication_check)


def cue(text="Hello there", start=1, end=2):
    return {"text": text, "start": start, "end": end}


def witness(probability=.95):
    return {"model": "independent", "segments": [{"words": [
        {"word": "Hello", "start": 1, "end": 1.4, "probability": probability},
        {"word": "there", "start": 1.5, "end": 2, "probability": probability}]}]}


class AutomaticPreparationTest(unittest.TestCase):
    def test_all_media_types_are_discovered_and_published_items_are_untouched(self):
        for kind in ("video", "music", "podcast", "story"):
            payload = {"published": False, "url": "/media/source.mp4"}
            self.assertEqual(next_stage(kind, payload), ("transcode", "") if kind in ("video", "music") else ("synchronize", ""))
            self.assertIsNone(next_stage(kind, {**payload, "published": True}))

    def test_pipeline_is_serial_and_stops_after_translation_exceptions(self):
        payload = {"published": False, "url": "/media/source.mp4", "hls_manifest_key": "hls/id/master.m3u8", "transcript": [cue()]}
        payload["hls_source"] = media_source(payload)
        self.assertEqual(next_stage("video", payload), ("synchronize", ""))
        payload["alignment_source"] = identity(payload)
        payload["automation"] = {"source": {"passed": True, "input": source_input(payload)}}
        payload["transcript"][0].update(editorial_review={"decision": "auto_approved"}, timing_status="auto_approved")
        self.assertEqual(next_stage("video", payload), ("translate", "fa"))
        payload["automation"]["translations"] = {"fa": {"passed": False, "input": translation_input(payload, "fa")}}
        self.assertEqual(next_stage("video", payload), ("translate", "ckb"))
        payload["automation"]["translations"]["ckb"] = {"passed": False, "input": translation_input(payload, "ckb")}
        self.assertIsNone(next_stage("video", payload))
        payload["transcript"][0]["text"] = "Changed words"
        self.assertEqual(next_stage("video", payload), ("synchronize", ""))

    def test_second_recognizer_disagreement_confidence_timing_and_missing_words_block(self):
        self.assertTrue(source_assessment([cue()], witness(), 10)["passed"])
        self.assertFalse(source_assessment([cue("Wrong words")], witness(), 10)["passed"])
        self.assertFalse(source_assessment([cue()], witness(.6), 10)["passed"])
        self.assertFalse(source_assessment([cue(start=5, end=6)], witness(), 10)["passed"])
        self.assertFalse(source_assessment([cue("Hello")], witness(), 10)["passed"])

    def test_invalid_timing_never_passes(self):
        for start, end in ((None, None), (float("nan"), 2), (-1, 2), (3, 2), (1, float("inf"))):
            self.assertFalse(source_assessment([cue(start=start, end=end)], witness(), 10)["passed"])

    def test_machine_acceptance_is_labelled_and_does_not_grant_rights_or_publish(self):
        original = {"published": False, "rights_approved": False, "transcript": [cue()]}
        result = {"automatic_source_check": source_assessment([cue()], witness(), 10)}
        updated = accept_source(original, result, "job")
        self.assertEqual(updated["transcript"][0]["editorial_review"]["decision"], "auto_approved")
        self.assertEqual(updated["transcript"][0]["exercise_review"], "auto_approved")
        self.assertEqual(updated["transcript"][0]["review_revision"], 1)
        self.assertFalse(updated["rights_approved"])
        self.assertFalse(updated["published"])
        self.assertNotIn("cue_id", original["transcript"][0])

    def test_alignment_warning_overrides_model_agreement(self):
        result = {"automatic_source_check": source_assessment([cue()], witness(), 10),
                  "exceptions": [{"reasons": ["possible_missing_words"]}]}
        updated = accept_source({"transcript": [cue()]}, result, "job")
        self.assertNotIn("editorial_review", updated["transcript"][0])
        self.assertFalse(updated["automation"]["source"]["passed"])

    def test_only_uncertain_lines_remain_unapproved(self):
        check = {"passed": False, "verification_model": "independent", "issues": [{"paragraph": 2, "reasons": ["low_recognition_confidence"]}]}
        updated = accept_source({"transcript": [cue(), cue("Other words", 3, 4)]}, {"automatic_source_check": check}, "job")
        self.assertEqual(updated["transcript"][0]["timing_status"], "auto_approved")
        self.assertNotIn("editorial_review", updated["transcript"][1])

    def test_translation_checks_reject_wrong_script_and_changed_meaning(self):
        self.assertTrue(translation_assessment("Hello there", "سلام دوست", "Hello there", independent=True)["passed"])
        self.assertFalse(translation_assessment("Hello there", "سلام دوست", "Hello there")["passed"])
        self.assertFalse(translation_assessment("Hello there", "Hello there", "Hello there")["passed"])
        self.assertFalse(translation_assessment("Hello there", "سلام دوست", "Very different meaning")["passed"])

    def test_automatic_translation_applies_once_and_preserves_human_approvals(self):
        payload = {"published": False, "transcript": [cue()]}
        result = {"translations": [{"paragraph": 1, "text": "Hello there", "translation": "سلام دوست", "automatic_check": {"passed": True, "independent_model": True}}]}
        updated = apply_translations(payload, result, "fa", "job")
        self.assertEqual(updated["translation_status"]["fa"], "auto_approved")
        again = apply_translations(updated, result, "fa", "job")
        self.assertEqual(again, updated)
        human = copy.deepcopy(updated)
        human["transcript"][0]["translation_review"]["fa"] = "approved"
        retained = apply_translations(human, result, "fa", "job")
        self.assertEqual(retained["transcript"][0]["translation_review"]["fa"], "approved")
        wrong = copy.deepcopy(result)
        wrong["translations"][0]["translation"] = "متفاوت"
        with self.assertRaisesRegex(ValueError, "overwrite"):
            apply_translations(human, wrong, "fa", "job")

    def test_bad_translation_is_applied_as_draft_with_exception(self):
        result = {"translations": [{"paragraph": 1, "text": "Hello there", "translation": "سلام", "automatic_check": {"passed": False}}]}
        updated = apply_translations({"transcript": [cue()]}, result, "fa", "job")
        self.assertEqual(updated["translation_status"]["fa"], "draft")
        self.assertEqual(updated["automation"]["translations"]["fa"]["issues"][0]["paragraph"], 1)

    def test_translation_source_mismatch_and_duplicate_rows_are_rejected(self):
        row = {"paragraph": 1, "text": "Wrong source", "translation": "سلام"}
        with self.assertRaises(ValueError):
            apply_translations({"transcript": [cue()]}, {"translations": [row]}, "fa", "job")
        row["text"] = "Hello there"
        with self.assertRaises(ValueError):
            apply_translations({"transcript": [cue()]}, {"translations": [row, row]}, "fa", "job")

    def test_source_hash_ignores_approval_metadata_but_changes_with_words(self):
        payload = {"language": "en", "transcript": [cue()]}
        hashed = translation_hash(payload)
        payload["rights_approved"] = True
        self.assertEqual(translation_hash(payload), hashed)
        payload["transcript"][0]["text"] = "Different"
        self.assertNotEqual(translation_hash(payload), hashed)

    def test_media_and_timing_edits_invalidate_previous_automatic_results(self):
        payload = {"published": False, "url": "/media/one.mp4", "hls_manifest_key": "hls/id/master.m3u8", "transcript": [cue()]}
        payload["hls_source"] = media_source(payload)
        checked = accept_source(payload, {"automatic_source_check": source_assessment([cue()], witness(), 10)}, "job")
        self.assertEqual(next_stage("video", checked), ("translate", "fa"))
        changed = copy.deepcopy(checked)
        changed["url"] = "/media/two.mp4"
        self.assertEqual(next_stage("video", changed), ("transcode", ""))
        changed = copy.deepcopy(checked)
        changed["transcript"][0]["start"] = 0
        self.assertEqual(next_stage("video", changed), ("synchronize", ""))

    def test_exception_result_does_not_cause_endless_synchronization(self):
        payload = {"published": False, "url": "/media/source.mp3", "transcript": [cue()]}
        result = {"automatic_source_check": source_assessment([cue()], witness(), 10),
                  "exceptions": [{"candidate_start": 0, "candidate_end": 1, "reasons": ["possible_missing_words"]}]}
        checked = accept_source(payload, result, "job")
        self.assertEqual(next_stage("podcast", checked), ("translate", "fa"))

    def test_publication_requires_opt_in_rights_and_current_evidence(self):
        payload = {"published": False, "auto_publish": True, "rights_approved": True,
                   "rights_reference": "licensed fixture", "url": "/media/test.wav", "language": "en",
                   "transcript": [cue()]}
        payload = accept_source(payload, {"automatic_source_check": source_assessment([cue()], witness(), 10)}, "source-job")
        for language in ("fa", "ckb"):
            payload = apply_translations(payload, {"translations": [{"paragraph": 1, "text": "Hello there",
                "translation": "سلام دوست", "automatic_check": {"passed": True,
                "independent_model": True}}]}, language, language + "-job")
        self.assertEqual(automatic_publication_check("podcast", payload), [])
        for key, value in (("auto_publish", False), ("rights_approved", False), ("rights_reference", "")):
            changed = copy.deepcopy(payload)
            changed[key] = value
            self.assertTrue(automatic_publication_check("podcast", changed))
        changed = copy.deepcopy(payload)
        changed["transcript"][0]["translation"]["ckb"] = "changed"
        self.assertIn("ckb_verification_required", automatic_publication_check("podcast", changed))
        changed = copy.deepcopy(payload)
        changed["transcript"][0]["start"] = 5
        self.assertIn("source_verification_required", automatic_publication_check("podcast", changed))
        changed = copy.deepcopy(payload)
        changed["transcript"].append(copy.deepcopy(changed["transcript"][0]))
        self.assertIn("stable_cue_ids_required", automatic_publication_check("podcast", changed))
        self.assertIn("hls_required", automatic_publication_check("music", payload))


if __name__ == "__main__":
    unittest.main()
