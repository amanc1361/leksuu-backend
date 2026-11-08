"""Produce versioned HLS renditions in a private media store.

Only the caller publishes the master key after all files are present. Output
paths are relative and never contain remote URLs or user supplied filenames.
"""

import json
import os
import shutil
import subprocess
from pathlib import Path


PROFILES = ((240, 350_000), (360, 650_000), (480, 1_100_000),
            (720, 2_300_000), (1080, 4_500_000))


def ffmpeg_binary():
    binary = os.environ.get("MEDIA_FFMPEG") or shutil.which("ffmpeg")
    if not binary:
        raise ValueError("ffmpeg_unavailable")
    return binary


def source_info(source):
    import av
    with av.open(str(source)) as container:
        video = next((stream for stream in container.streams if stream.type == "video"), None)
        audio = next((stream for stream in container.streams if stream.type == "audio"), None)
        if not video and not audio:
            raise ValueError("media_has_no_audio_or_video")
        return {"width": video.width if video else 0,
                "height": video.height if video else 0,
                "audio": audio is not None,
                "duration": float(container.duration or 0) / 1_000_000}


def profiles_for(info):
    if not info["height"]:
        return []
    height = info["height"]
    choices = [(h, bitrate) for h, bitrate in PROFILES if h <= height]
    if not choices:
        choices = [(max(2, height - height % 2), 250_000)]
    return choices


def run_ffmpeg(source, folder, binary=None):
    binary = binary or ffmpeg_binary()
    folder = Path(folder)
    folder.mkdir(parents=True, exist_ok=True)
    info = source_info(source)
    profiles = profiles_for(info)
    variants = []
    if not profiles:
        target = folder / "audio"
        target.mkdir()
        command = [binary, "-hide_banner", "-nostdin", "-y", "-i", str(source),
                   "-vn", "-c:a", "aac", "-b:a", "128k", "-f", "hls",
                   "-hls_time", "4", "-hls_playlist_type", "vod",
                   "-hls_flags", "independent_segments+temp_file",
                   "-hls_segment_filename", str(target / "seg_%05d.ts"),
                   str(target / "index.m3u8")]
        _execute(command)
        variants.append(("audio/index.m3u8", 160_000, None))
    for height, bitrate in profiles:
        target = folder / f"v{height}"
        target.mkdir()
        command = [binary, "-hide_banner", "-nostdin", "-y", "-i", str(source),
                   "-map", "0:v:0", "-map", "0:a:0?", "-vf", f"scale=-2:{height}",
                   "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p",
                   "-b:v", str(bitrate), "-maxrate", str(int(bitrate * 1.3)),
                   "-bufsize", str(bitrate * 2), "-g", "48", "-keyint_min", "48",
                   "-sc_threshold", "0", "-c:a", "aac", "-b:a", "128k",
                   "-f", "hls", "-hls_time", "4", "-hls_playlist_type", "vod",
                   "-hls_flags", "independent_segments+temp_file",
                   "-hls_segment_filename", str(target / "seg_%05d.ts"),
                   str(target / "index.m3u8")]
        _execute(command)
        width = max(2, round(info["width"] * height / info["height"] / 2) * 2)
        variants.append((f"v{height}/index.m3u8", bitrate + (128_000 if info["audio"] else 0), (width, height)))
    master = ["#EXTM3U", "#EXT-X-VERSION:3"]
    for playlist, bandwidth, resolution in variants:
        attributes = f"BANDWIDTH={bandwidth}"
        if resolution:
            attributes += f",RESOLUTION={resolution[0]}x{resolution[1]}"
        master += [f"#EXT-X-STREAM-INF:{attributes}", playlist]
    (folder / "master.m3u8").write_text("\n".join(master) + "\n", encoding="utf-8")
    return {"profiles": [entry[0] for entry in variants], "duration_seconds": info["duration"]}


def _execute(command):
    result = subprocess.run(command, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
                            timeout=int(os.environ.get("MEDIA_FFMPEG_TIMEOUT", "7200")), check=False)
    if result.returncode:
        raise ValueError("ffmpeg_transcode_failed")


def upload_hls(folder, prefix):
    """Stage children first, then master. A failed upload cannot publish a manifest."""
    folder = Path(folder)
    files = sorted(p for p in folder.rglob("*") if p.is_file() and p.name != "master.m3u8")
    files.append(folder / "master.m3u8")
    if os.environ.get("S3_BUCKET"):
        import boto3
        endpoint = os.environ.get("S3_ENDPOINT", "")
        if endpoint and not endpoint.startswith(("https://", "http://")):
            endpoint = ("https://" if os.environ.get("S3_SECURE", "true") == "true" else "http://") + endpoint
        client = boto3.client("s3", endpoint_url=endpoint or None,
                              region_name=os.environ.get("S3_REGION", "us-east-1"),
                              aws_access_key_id=os.environ["S3_ACCESS_KEY"],
                              aws_secret_access_key=os.environ["S3_SECRET_KEY"])
        bucket = os.environ["S3_BUCKET"]
        storage_prefix = os.environ.get("S3_PREFIX", "").strip("/")
        for file in files:
            key = "/".join(part for part in (storage_prefix, prefix, file.relative_to(folder).as_posix()) if part)
            client.upload_file(str(file), bucket, key, ExtraArgs={
                "ContentType": "application/vnd.apple.mpegurl" if file.suffix == ".m3u8" else "video/mp2t",
                "CacheControl": "private, no-store"})
    else:
        root = os.environ.get("MEDIA_JOBS_LOCAL_MEDIA_DIR")
        if not root:
            raise ValueError("private_media_store_unavailable")
        for file in files:
            target = Path(root) / prefix / file.relative_to(folder)
            target.parent.mkdir(parents=True, exist_ok=True)
            temporary = target.with_name(target.name + ".tmp")
            shutil.copyfile(file, temporary)
            os.replace(temporary, target)
    return prefix + "/master.m3u8"
