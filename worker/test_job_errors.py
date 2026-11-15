import unittest
from job_errors import failure_code


class JobErrorTests(unittest.TestCase):
    def test_missing_translation_dependencies_have_actionable_safe_code(self):
        self.assertEqual(failure_code(ModuleNotFoundError("private server path"), "translate"),
                         "translation_dependency_unavailable")
        self.assertEqual(failure_code(ImportError("private server path"), "synchronize"),
                         "processing_dependency_unavailable")

    def test_preserves_known_errors_and_does_not_expose_exception_details(self):
        self.assertEqual(failure_code(ValueError("translation_model_unavailable"), "translate"),
                         "translation_model_unavailable")
        self.assertEqual(failure_code(TimeoutError(), "translate"), "provider_timeout")
        self.assertEqual(failure_code(ConnectionError(), "translate"), "provider_unavailable")
        self.assertEqual(failure_code(MemoryError(), "translate"), "out_of_memory")
        self.assertEqual(failure_code(RuntimeError("secret details"), "translate"), "processing_failed")
        self.assertEqual(failure_code(ValueError("/private/path"), "translate"), "processing_failed")


if __name__ == "__main__":
    unittest.main()
