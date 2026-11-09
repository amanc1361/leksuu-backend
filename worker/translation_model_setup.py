"""Explicit model-volume setup. Never imports the worker or connects to a DB.

Default is read-only check. Installation is opt-in and will not replace existing
files. This uses the same pinned model as story-sync/download_translation_model.py.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys
import urllib.request

REVISION = "aa32bbdeba7880eff2096ec044cb155a340a9400"
BASE = f"https://huggingface.co/Nextcloud-AI/madlad400-3b-mt-ct2-int8/resolve/{REVISION}"
FILES = {
    "model.bin": (2950208329, "77b9fd9ab97c1259d07089b5f854393dad81bc5fb5647d3f9a5d101c94f40daa"),
    "spiece.model": (4427844, "ef11ac9a22c7503492f56d48dce53be20e339b63605983e9f27d2cd0e0f3922c"),
    "config.json": (224, "90fb54962455a4e0a0bc7235c0f063d7e46d9c1a1ae003af8059809abd6aeece"),
    "shared_vocabulary.json": (5477099, "c327551ce3ca6efc7b437e11a267f79979893332dda8a1d146e2c950815193f8"),
}


def verified(path, size, digest):
    if not path.is_file() or path.is_symlink() or path.stat().st_size != size:
        return False
    with path.open("rb") as handle:
        return hashlib.file_digest(handle, "sha256").hexdigest() == digest


def check(directory, files=FILES):
    return {name: ("missing" if not (directory / name).exists() else
                   "verified" if verified(directory / name, *spec) else "different_or_incomplete")
            for name, spec in files.items()}


def install(directory, files=FILES, base=BASE, opener=urllib.request.urlopen):
    # Fail before any download when the destination contains a different model.
    states = check(directory, files)
    conflicts = [name for name, state in states.items() if state == "different_or_incomplete"]
    if conflicts:
        raise ValueError("existing_model_files_left_unchanged: " + ", ".join(conflicts))
    directory.mkdir(parents=True, exist_ok=True)
    if all(state == "verified" for state in states.values()):
        return
    required = sum(size for name, (size, _) in files.items() if states[name] != "verified")
    if shutil.disk_usage(directory).free < required + (64 << 20):
        raise ValueError("insufficient_model_volume_space")
    lock = directory / ".leksuu-translation-install.lock"
    descriptor = os.open(lock, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    os.close(descriptor)
    try:
        for name, (size, digest) in files.items():
            target = directory / name
            if verified(target, size, digest):
                continue
            partial = directory / (".leksuu-install-" + name + ".part")
            if partial.is_symlink():
                raise ValueError("invalid_partial_file")
            offset = partial.stat().st_size if partial.exists() else 0
            if offset > size:
                raise ValueError("oversized_partial_left_unchanged")
            if offset < size:
                request = urllib.request.Request(base + "/" + name,
                                                headers={"Range": f"bytes={offset}-"} if offset else {})
                with opener(request, timeout=60) as response:
                    if offset and response.status == 206:
                        if not response.headers.get("Content-Range", "").startswith(f"bytes {offset}-"):
                            raise ValueError("invalid_download_range")
                    else:
                        if response.status != 200:
                            raise ValueError("unexpected_download_status")
                        offset = 0
                    with partial.open("ab" if offset else "wb") as output:
                        received, next_report = offset, offset + (64 << 20)
                        while True:
                            chunk = response.read(1 << 20)
                            if not chunk:
                                break
                            if received + len(chunk) > size:
                                raise ValueError("download_exceeds_pinned_size")
                            output.write(chunk)
                            received += len(chunk)
                            if received >= next_report:
                                print(json.dumps({"file": name, "bytes": received, "total": size}), flush=True)
                                next_report = received + (64 << 20)
            if not verified(partial, size, digest):
                raise ValueError("download_incomplete_or_checksum_mismatch: " + name)
            # Atomic, same-volume publication without overwriting an existing file.
            os.link(partial, target)
            partial.unlink()
            print(json.dumps({"file": name, "status": "verified"}), flush=True)
    finally:
        lock.unlink()


def smoke_test(directory):
    import ctranslate2
    import sentencepiece
    tokenizer = sentencepiece.SentencePieceProcessor(model_file=str(directory / "spiece.model"))
    translator = ctranslate2.Translator(str(directory), device="cpu", compute_type="int8",
                                        inter_threads=1, intra_threads=4)
    for target in ("fa", "ckb"):
        if tokenizer.piece_to_id(f"<2{target}>") == tokenizer.unk_id():
            raise ValueError("unsupported_translation_target: " + target)
        tokens = tokenizer.encode(f"<2{target}> Hello, how are you?", out_type=str)
        result = translator.translate_batch([tokens], beam_size=2, max_decoding_length=64)
        text = tokenizer.decode([piece for piece in result[0].hypotheses[0] if piece not in ("</s>", "<pad>")])
        if sum(0x0600 <= ord(c) <= 0x08FF for c in text) < 2:
            raise ValueError("translation_smoke_test_failed: " + target)
        print(json.dumps({"target": target, "status": "synthetic_translation_passed"}), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", default=os.environ.get("MEDIA_JOBS_TRANSLATION_MODEL_DIR"))
    action = parser.add_mutually_exclusive_group()
    action.add_argument("--install", action="store_true", help="Download about 2.96 GB into the existing model volume")
    action.add_argument("--check", action="store_true", help="Read-only file size and SHA-256 checks (default)")
    action.add_argument("--smoke-test", action="store_true", help="Check files and run synthetic Persian/Sorani inference; requires RAM")
    args = parser.parse_args()
    if not args.directory:
        parser.error("Set MEDIA_JOBS_TRANSLATION_MODEL_DIR or --directory; refusing to use the working directory")
    directory = Path(args.directory).resolve()
    if directory == Path(directory.anchor):
        parser.error("Refusing to use a filesystem root as the model directory")
    if args.install:
        install(directory)
    states = check(directory)
    print(json.dumps({"model_revision": REVISION, "files": states}), flush=True)
    if any(state != "verified" for state in states.values()):
        return 1
    if args.smoke_test:
        smoke_test(directory)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as error:
        print(json.dumps({"status": "failed", "reason": str(error)}), file=sys.stderr)
        sys.exit(1)
