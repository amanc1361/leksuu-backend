import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(tempfile.gettempdir()) / "leksuu-align-pydeps"))

import imageio_ffmpeg

from worker.hls_pipeline import _execute, run_ffmpeg, upload_hls


class HlsPipelineTest(unittest.TestCase):
    def test_real_audio_only_music_rendition(self):
        binary = imageio_ffmpeg.get_ffmpeg_exe()
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            source = root / "music.m4a"
            _execute([binary, "-hide_banner", "-y", "-f", "lavfi", "-i",
                      "sine=frequency=523:sample_rate=48000", "-t", "3", "-c:a", "aac", str(source)])
            output = root / "hls"
            result = run_ffmpeg(source, output, binary)
            self.assertEqual(result["profiles"], ["audio/index.m3u8"])
            self.assertIn("audio/index.m3u8", (output / "master.m3u8").read_text())
            playlist = (output / "audio/index.m3u8").read_text()
            self.assertIn("#EXT-X-ENDLIST", playlist)
            self.assertTrue(any((output / "audio" / line).is_file() for line in playlist.splitlines()
                                if line and not line.startswith("#")))

    def test_real_video_renditions_and_private_upload(self):
        binary = imageio_ffmpeg.get_ffmpeg_exe()
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            source = root / "fixture.mp4"
            _execute([binary, "-hide_banner", "-y", "-f", "lavfi", "-i",
                      "testsrc=size=640x360:rate=24", "-f", "lavfi", "-i",
                      "sine=frequency=440:sample_rate=48000", "-t", "4",
                      "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", str(source)])
            output = root / "hls"
            result = run_ffmpeg(source, output, binary)
            self.assertIn("v360/index.m3u8", result["profiles"])
            master = (output / "master.m3u8").read_text()
            self.assertIn("#EXT-X-STREAM-INF", master)
            for variant in result["profiles"]:
                playlist = (output / variant).read_text()
                self.assertIn("#EXT-X-ENDLIST", playlist)
                for line in playlist.splitlines():
                    if line and not line.startswith("#"):
                        self.assertTrue((output / Path(variant).parent / line).is_file())
            with patch.dict(os.environ, {"MEDIA_JOBS_LOCAL_MEDIA_DIR": str(root / "store"), "S3_BUCKET": ""}):
                key = upload_hls(output, "hls/test-item/version")
            self.assertEqual(key, "hls/test-item/version/master.m3u8")
            self.assertTrue((root / "store" / key).is_file())


if __name__ == "__main__":
    unittest.main()
