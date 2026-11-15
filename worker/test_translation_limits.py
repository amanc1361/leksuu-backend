"""Translation limit regression without model or database dependencies."""
import ast
import os
import re
import unittest
from pathlib import Path


class TranslationLimitsTest(unittest.TestCase):
    def test_long_unpunctuated_cue_keeps_every_word(self):
        tree = ast.parse(Path(__file__).with_name("leksuu_media_worker.py").read_text(encoding="utf-8"))
        function = next(node for node in tree.body if isinstance(node, ast.FunctionDef)
                        and node.name == "translation_chunks")
        scope = {"re": re}
        exec(compile(ast.Module(body=[function], type_ignores=[]), "worker", "exec"), scope)
        source = " ".join(f"word{index}" for index in range(245))
        chunks = scope["translation_chunks"](source)
        self.assertEqual([len(chunk.split()) for chunk in chunks], [110, 110, 25])
        self.assertEqual(" ".join(chunks), source)

    def test_episode_translation_preserves_existing_drafts(self):
        tree = ast.parse(Path(__file__).with_name("leksuu_media_worker.py").read_text(encoding="utf-8"))
        functions = [node for node in tree.body if isinstance(node, ast.FunctionDef)
                     and node.name in ("translation_source", "process_translation")]
        scope = {"os": os, "re": re, "automation_enabled": lambda: False}
        exec(compile(ast.Module(body=functions, type_ignores=[]), "worker", "exec"), scope)
        payload = {"transcript": [{"text": "Reviewed source", "translation": {"fa": "existing"}}
                                  for _ in range(419)]}
        job = {"payload_snapshot": payload, "target_language": "fa", "payload_sha256": "source"}
        result = scope["process_translation"](job, payload)
        self.assertEqual(result["generated"], 0)
        payload["transcript"] *= 5
        with self.assertRaisesRegex(ValueError, "translation_cost_limit"):
            scope["process_translation"](job, payload)


if __name__ == "__main__":
    unittest.main()
