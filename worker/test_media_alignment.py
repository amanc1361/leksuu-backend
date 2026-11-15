"""Focused alignment tests independent of database and model inference."""

import unittest
import sys
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))

from leksuu_media_worker import align, detect_spoken_language, exact_word_timing, merge_synchronization, needs_story_synchronization, process


def witness(*entries):
    return {"segments": [{"words": [{"word": word, "start": start, "end": end}
                                    for word, start, end in entries]}]}


class AlignmentTest(unittest.TestCase):
    def test_language_detection_requires_multilingual_model_and_confidence(self):
        from types import SimpleNamespace
        self.assertFalse(detect_spoken_language("unused.mp3", "en")["passed"])
        with patch("leksuu_media_worker.WhisperModel", side_effect=OSError("not installed")), \
             patch.dict("os.environ", {"MEDIA_JOBS_LANGUAGE_DETECTION_MODEL": "missing-multilingual"}):
            self.assertEqual(detect_spoken_language("unused.mp3", "en")["reason"],
                             "language_detection_model_unavailable")
        class Model:
            def transcribe(self, *_args, **_kwargs):
                return iter(()), SimpleNamespace(language="en", language_probability=.91)
        with patch("leksuu_media_worker.LANGUAGE_MODEL", Model()), \
             patch("leksuu_media_worker.LANGUAGE_MODEL_NAME", "test-multilingual"), \
             patch.dict("os.environ", {"MEDIA_JOBS_LANGUAGE_DETECTION_MODEL": "test-multilingual"}), \
             patch("leksuu_media_worker.decode_audio", return_value=[1] * 16000):
            self.assertTrue(detect_spoken_language("unused.mp3", "en")["passed"])
            self.assertFalse(detect_spoken_language("unused.mp3", "fa")["passed"])
        class GenericKurdishModel:
            def transcribe(self, *_args, **_kwargs):
                return iter(()), SimpleNamespace(language="ku", language_probability=.99)
        with patch("leksuu_media_worker.LANGUAGE_MODEL", GenericKurdishModel()), \
             patch("leksuu_media_worker.LANGUAGE_MODEL_NAME", "test-multilingual"), \
             patch.dict("os.environ", {"MEDIA_JOBS_LANGUAGE_DETECTION_MODEL": "test-multilingual"}), \
             patch("leksuu_media_worker.decode_audio", return_value=[1] * 16000):
            result = detect_spoken_language("unused.mp3", "ckb")
            self.assertFalse(result["passed"])
            self.assertEqual(result["detected"], "ku")

    def test_full_recording_chunks_keep_late_lyrics_without_vad(self):
        from types import SimpleNamespace
        from leksuu_media_worker import transcribe
        class Audio:
            def __len__(self): return 125 * 16000
            def __getitem__(self, key): return key
        calls = []
        class Model:
            def transcribe(self, audio, **options):
                calls.append((audio, options))
                # Relative timestamp in each overlapping window.
                word = SimpleNamespace(start=4, end=5, word=" Late", probability=.9)
                return iter([SimpleNamespace(words=[word])]), None
        with patch("leksuu_media_worker.MODEL", Model()), \
             patch("leksuu_media_worker.MODEL_NAME", "test-model"), \
             patch.dict("os.environ", {"MEDIA_JOBS_WHISPER_MODEL": "test-model"}), \
             patch("leksuu_media_worker.decode_audio", return_value=Audio()):
            result = transcribe("unused.mkv", "en")
        self.assertEqual(len(calls), 3)
        self.assertTrue(all(not options["vad_filter"] for _, options in calls))
        self.assertEqual([line["start"] for line in result["segments"]], [4, 62, 122])
        self.assertEqual(result["duration_seconds"], 125)
        self.assertEqual(result["segments"][-1]["text"], "Late")

    def test_instrumental_gap_splits_generated_lyric_lines(self):
        from types import SimpleNamespace
        from leksuu_media_worker import transcribe
        class Audio:
            def __len__(self): return 50 * 16000
            def __getitem__(self, key): return key
        class Model:
            def transcribe(self, *_args, **_kwargs):
                timed = [SimpleNamespace(start=i, end=i+.5, word=" lyric", probability=.9)
                         for i in [1, 2, 3, 40, 41, 42]]
                return iter([SimpleNamespace(words=timed)]), None
        with patch("leksuu_media_worker.MODEL", Model()), \
             patch("leksuu_media_worker.MODEL_NAME", "test-model"), \
             patch.dict("os.environ", {"MEDIA_JOBS_WHISPER_MODEL": "test-model"}), \
             patch("leksuu_media_worker.decode_audio", return_value=Audio()):
            result = transcribe("unused.mp3", "en")
        self.assertEqual([(line["start"], line["end"]) for line in result["segments"]],
                         [(1, 3.5), (40, 42.5)])

    def test_asr_window_checkpoint_resumes_without_reprocessing_completed_audio(self):
        from types import SimpleNamespace
        from leksuu_media_worker import transcribe
        class Audio:
            def __len__(self): return 125 * 16000
            def __getitem__(self, key): return key
        calls = []
        class Model:
            def transcribe(self, audio, **_options):
                calls.append(audio)
                return iter([SimpleNamespace(words=[SimpleNamespace(
                    start=4, end=5, word=" Hello", probability=.95)])]), None
        checkpoints = []
        def interrupt_after_first(checkpoint):
            checkpoints.append(checkpoint)
            raise RuntimeError("worker stopped")
        with patch("leksuu_media_worker.MODEL", Model()), \
             patch("leksuu_media_worker.MODEL_NAME", "test-model"), \
             patch.dict("os.environ", {"MEDIA_JOBS_WHISPER_MODEL": "test-model"}), \
             patch("leksuu_media_worker.decode_audio", return_value=Audio()):
            with self.assertRaisesRegex(RuntimeError, "worker stopped"):
                transcribe("unused.mp3", "en", on_window=interrupt_after_first)
            resumed = transcribe("unused.mp3", "en", checkpoint=checkpoints[0])
        self.assertEqual(len(calls), 3)
        self.assertEqual(resumed["completed_windows"], 3)
        self.assertEqual([line["start"] for line in resumed["segments"]], [4, 62, 122])

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

    def test_authorized_embedded_cues_keep_original_timing_after_asr_comparison(self):
        payload = {"url": "http://local.invalid/media/film.mp4", "language": "en",
                   "rights_approved": True, "rights_reference": "fixture-license", "transcript": []}
        job = {"id": "job", "item_id": "item", "kind": "video", "operation": "synchronize",
               "payload_snapshot": payload, "result": None}
        class Connection:
            def __enter__(self): return self
            def __exit__(self, *_): return False
            def execute(self, statement, *_):
                if statement.startswith("SELECT"):
                    class Row:
                        def fetchone(self): return {"kind": "video", "payload": payload}
                    return Row()
                return None
        spoken = witness(("Hello", 1.0, 1.3), ("there", 1.4, 1.9))
        spoken["segments"][0].update(start=1.0, end=1.9)
        with patch("leksuu_media_worker.db", return_value=Connection()), \
             patch("leksuu_media_worker.fetch_original", return_value=(Path("unused.mp4"), "a" * 64)), \
             patch("leksuu_media_worker.audio_duration", return_value=4.0), \
             patch("leksuu_media_worker.opening_sound_onset", return_value=0.0), \
             patch("leksuu_media_worker.transcribe", return_value=spoken), \
             patch("leksuu_media_worker.automation_enabled", return_value=False), \
             patch("leksuu_media_worker.extract_embedded_subtitles", return_value=(
                 [{"text": "Hello there", "start": .8, "end": 2.0}],
                 {"method": "embedded_subtitle_track", "stream_index": 2})):
            result = process(job)
        self.assertEqual(result["method"], "embedded_subtitles_verified_with_asr")
        self.assertEqual(result["subtitle_provenance"]["stream_index"], 2)
        self.assertEqual((result["cues"][0]["start"], result["cues"][0]["end"]), (.8, 2.0))


if __name__ == "__main__":
    unittest.main()
