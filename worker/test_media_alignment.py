"""Focused alignment tests independent of database and model inference."""

import unittest
import sys
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))

from leksuu_media_worker import align, process


def witness(*entries):
    return {"segments": [{"words": [{"word": word, "start": start, "end": end}
                                    for word, start, end in entries]}]}


class AlignmentTest(unittest.TestCase):
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


if __name__ == "__main__":
    unittest.main()
