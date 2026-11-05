"""Focused alignment tests independent of database and model inference."""

import unittest
import sys
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))

from leksuu_media_worker import align, exact_word_timing, merge_synchronization, needs_story_synchronization, process


def witness(*entries):
    return {"segments": [{"words": [{"word": word, "start": start, "end": end}
                                    for word, start, end in entries]}]}


class AlignmentTest(unittest.TestCase):
    def test_story_reconciliation_requires_exact_word_timing_once(self):
        payload = {"url": "/media/story.mp3", "language": "en", "audio_sha256": "a" * 64,
                   "transcript": [{"text": "Two words.", "start": 1.0, "end": 2.0}]}
        self.assertTrue(needs_story_synchronization(payload))
        payload["transcript"][0]["words"] = [
            {"word": "Two", "start": 1.0, "end": 1.3},
            {"word": "words", "start": 1.4, "end": 1.9},
        ]
        self.assertTrue(exact_word_timing(payload["transcript"][0]))
        self.assertFalse(needs_story_synchronization(payload))

    def test_completed_alignment_version_is_not_requeued_for_review(self):
        payload = {"url": "/media/story.mp3", "language": "en", "audio_sha256": "a" * 64,
                   "transcript": [{"text": "Uncertain words.", "timing_status": "unverified"}]}
        payload["alignment_version"] = 2
        payload["alignment_source"] = {"url": payload["url"], "audio_sha256": payload["audio_sha256"],
                                       "language": "en", "transcript_texts": ["Uncertain words."]}
        self.assertFalse(needs_story_synchronization(payload))

    def test_unspoken_introduction_does_not_shift_narrative(self):
        transcript = [{"text": "We present this story today."}, {"text": "The house was dark."}]
        cues, exceptions, extra, _, _ = align(transcript,
            witness(("The", 2, 2.2), ("house", 2.3, 2.6), ("was", 2.7, 2.8), ("dark", 2.9, 3.2)), 10, 0)
        self.assertEqual(cues[0]["timing_status"], "unverified")
        self.assertEqual(cues[1]["start"], 2)
        self.assertEqual(exceptions[0]["paragraph"], 1)
        self.assertEqual(exceptions[0]["matched_words"], 0)
        self.assertEqual(exceptions[0]["source_words"], 5)
        self.assertEqual(extra, [])

    def test_spoken_introduction_is_timed(self):
        transcript = [{"text": "We present this story today."}, {"text": "The house was dark."}]
        entries = [(word, i, i + 0.2) for i, word in enumerate("We present this story today The house was dark".split())]
        cues, exceptions, extra, _, _ = align(transcript, witness(*entries), 20, 0)
        self.assertEqual(len(exceptions), 0)
        self.assertEqual(cues[0]["start"], 0)
        self.assertEqual(cues[1]["start"], 5)
        self.assertEqual(cues[0]["words"][1], {"word": "present", "start": 1, "end": 1.2})
        self.assertEqual(len(cues[1]["words"]), 4)
        self.assertEqual(extra, [])

    def test_missing_spoken_content_is_reported(self):
        transcript = [{"text": "The house was dark."}]
        entries = [(word, i, i + 0.2) for i, word in enumerate("The house was dark something else was spoken here today".split())]
        _, _, extra, _, _ = align(transcript, witness(*entries), 20, 0)
        self.assertEqual(extra[0]["words"], 6)
        self.assertEqual(extra[0]["start"], 4)

    def test_existing_timing_is_compared_with_speech_not_trusted_by_presence(self):
        payload = {"url": "http://local.invalid/media/story.mp3", "language": "en",
                   "transcript": [{"text": "The house was dark.", "start": 1.0, "end": 2.0}]}
        job = {"id": "job", "item_id": "item", "kind": "story", "operation": "synchronize",
               "payload_snapshot": payload, "result": None}
        class Connection:
            def __enter__(self): return self
            def __exit__(self, *_): return False
            def execute(self, statement, *_):
                if statement.startswith("SELECT"):
                    class Row:
                        def fetchone(self): return {"kind": "story", "payload": payload}
                    return Row()
                return None
        spoken = witness(("The", 5.0, 5.2), ("house", 5.3, 5.5), ("was", 5.6, 5.7), ("dark", 5.8, 6.0))
        with patch("leksuu_media_worker.db", return_value=Connection()), \
             patch("leksuu_media_worker.fetch_original", return_value=(Path("unused.mp3"), "a" * 64)), \
             patch("leksuu_media_worker.audio_duration", return_value=10.0), \
             patch("leksuu_media_worker.opening_sound_onset", return_value=0.0), \
             patch("leksuu_media_worker.transcribe", return_value=spoken):
            result = process(job)
        self.assertEqual(result["method"], "validated_existing_cues_with_asr")
        self.assertEqual(result["cues"][0]["start"], 1.0)
        self.assertIn("existing_timing_disagrees_with_asr", result["exceptions"][0]["reasons"])
        self.assertFalse(result["human_editorial_approved"])

    def test_matching_existing_paragraph_timing_gains_exact_words(self):
        payload = {"url": "http://local.invalid/media/story.mp3", "language": "en",
                   "transcript": [{"text": "The house was dark.", "start": 1.0, "end": 2.0}]}
        job = {"id": "job", "item_id": "item", "kind": "story", "operation": "synchronize",
               "payload_snapshot": payload, "result": None}
        class Connection:
            def __enter__(self): return self
            def __exit__(self, *_): return False
            def execute(self, statement, *_):
                if statement.startswith("SELECT"):
                    class Row:
                        def fetchone(self): return {"kind": "story", "payload": payload}
                    return Row()
                return None
        spoken = witness(("The", 1.0, 1.15), ("house", 1.2, 1.4),
                         ("was", 1.5, 1.6), ("dark", 1.7, 1.95))
        with patch("leksuu_media_worker.db", return_value=Connection()), \
             patch("leksuu_media_worker.fetch_original", return_value=(Path("unused.mp3"), "a" * 64)), \
             patch("leksuu_media_worker.audio_duration", return_value=10.0), \
             patch("leksuu_media_worker.opening_sound_onset", return_value=0.0), \
             patch("leksuu_media_worker.transcribe", return_value=spoken):
            result = process(job)
        self.assertEqual(result["exceptions"], [])
        self.assertEqual(len(result["cues"][0]["words"]), 4)
        self.assertEqual(result["cues"][0]["start"], 1.0)
        self.assertEqual(result["cues"][0]["end"], 2.0)
        updated, unresolved = merge_synchronization(payload, result, "job")
        self.assertFalse(unresolved)
        self.assertEqual(len(updated["transcript"][0]["words"]), 4)
        self.assertEqual(updated["transcript"][0]["start"], 1.0)
        self.assertEqual(updated["alignment_version"], 2)
        self.assertEqual(updated["timing_status"], "ready")


if __name__ == "__main__":
    unittest.main()
