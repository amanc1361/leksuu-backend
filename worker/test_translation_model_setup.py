import hashlib
import io
from pathlib import Path
import tempfile
import unittest
from translation_model_setup import check, install


class Reply(io.BytesIO):
    def __init__(self, data, status=200, headers=None):
        super().__init__(data)
        self.status, self.headers = status, headers or {}


class SetupTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "model"
        self.data = b"small pinned synthetic model"
        self.files = {"model.bin": (len(self.data), hashlib.sha256(self.data).hexdigest())}

    def test_check_is_read_only(self):
        self.assertEqual(check(self.root, self.files), {"model.bin": "missing"})
        self.assertFalse(self.root.exists())

    def test_download_verifies_then_publishes_and_is_idempotent(self):
        calls = []
        def fetch(request, timeout):
            calls.append(request.full_url)
            return Reply(self.data)
        install(self.root, self.files, "https://fixture.invalid/pinned", fetch)
        self.assertEqual(check(self.root, self.files), {"model.bin": "verified"})
        install(self.root, self.files, "https://fixture.invalid/pinned", fetch)
        self.assertEqual(len(calls), 1)
        self.assertFalse((self.root / ".leksuu-translation-install.lock").exists())

    def test_existing_different_model_and_unrelated_files_are_never_replaced(self):
        self.root.mkdir()
        existing = self.root / "model.bin"
        existing.write_bytes(b"existing model")
        other = self.root / "other-model.bin"
        other.write_bytes(b"existing unrelated model")
        with self.assertRaisesRegex(ValueError, "existing_model_files_left_unchanged"):
            install(self.root, self.files, opener=lambda *a, **kw: self.fail("must not download"))
        self.assertEqual(existing.read_bytes(), b"existing model")
        self.assertEqual(other.read_bytes(), b"existing unrelated model")

    def test_resumes_only_installer_partial_and_checks_range(self):
        self.root.mkdir()
        partial = self.root / ".leksuu-install-model.bin.part"
        partial.write_bytes(self.data[:7])
        def fetch(request, timeout):
            self.assertEqual(request.get_header("Range"), "bytes=7-")
            return Reply(self.data[7:], 206, {"Content-Range": f"bytes 7-{len(self.data)-1}/{len(self.data)}"})
        install(self.root, self.files, opener=fetch)
        self.assertEqual((self.root / "model.bin").read_bytes(), self.data)

    def test_ignored_range_restarts_only_partial(self):
        self.root.mkdir()
        (self.root / ".leksuu-install-model.bin.part").write_bytes(self.data[:5])
        install(self.root, self.files, opener=lambda *a, **kw: Reply(self.data))
        self.assertEqual((self.root / "model.bin").read_bytes(), self.data)

    def test_corrupt_download_never_becomes_model(self):
        with self.assertRaisesRegex(ValueError, "checksum_mismatch"):
            install(self.root, self.files, opener=lambda *a, **kw: Reply(b"x" * len(self.data)))
        self.assertFalse((self.root / "model.bin").exists())
        self.assertFalse((self.root / ".leksuu-translation-install.lock").exists())

    def test_bad_range_cannot_append_wrong_bytes(self):
        self.root.mkdir()
        partial = self.root / ".leksuu-install-model.bin.part"
        partial.write_bytes(self.data[:5])
        with self.assertRaisesRegex(ValueError, "invalid_download_range"):
            install(self.root, self.files, opener=lambda *a, **kw: Reply(self.data, 206, {"Content-Range": "bytes 0-9/10"}))
        self.assertEqual(partial.read_bytes(), self.data[:5])


if __name__ == "__main__":
    unittest.main()
