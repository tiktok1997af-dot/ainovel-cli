#!/usr/bin/env python3
"""Create a content-free W6A determinism fingerprint for one GoReleaser snapshot."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import struct
import tarfile
import zipfile
from pathlib import Path


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def gzip_header_mtime(path: Path) -> int | None:
    with path.open("rb") as fh:
        header = fh.read(10)
    if len(header) < 10 or header[:2] != b"\x1f\x8b":
        return None
    return struct.unpack("<I", header[4:8])[0]


def tar_members(path: Path) -> list[dict[str, object]]:
    result: list[dict[str, object]] = []
    with tarfile.open(path, "r:gz") as tf:
        for member in tf.getmembers():
            if not member.isfile():
                continue
            result.append(
                {
                    "name": member.name.replace("\\", "/").lstrip("./"),
                    "size": member.size,
                    "mode": oct(member.mode),
                    "mtime": int(member.mtime),
                    "uid": member.uid,
                    "gid": member.gid,
                }
            )
    return sorted(result, key=lambda item: str(item["name"]))


def zip_members(path: Path) -> list[dict[str, object]]:
    result: list[dict[str, object]] = []
    with zipfile.ZipFile(path) as zf:
        for info in zf.infolist():
            if info.is_dir():
                continue
            result.append(
                {
                    "name": info.filename.replace("\\", "/").lstrip("./"),
                    "size": info.file_size,
                    "compress_size": info.compress_size,
                    "date_time": list(info.date_time),
                    "external_attr": info.external_attr,
                    "create_system": info.create_system,
                }
            )
    return sorted(result, key=lambda item: str(item["name"]))


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dist", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    dist = Path(args.dist).resolve()
    if not dist.is_dir():
        raise SystemExit(f"missing dist directory: {dist}")

    binaries: list[dict[str, object]] = []
    for path in sorted(dist.glob("ainovel-cli_*/*")):
        if not path.is_file() or path.name not in {"ainovel-cli", "ainovel-cli.exe"}:
            continue
        binaries.append(
            {
                "path": path.relative_to(dist).as_posix(),
                "size": path.stat().st_size,
                "sha256": sha256(path),
            }
        )

    archives: list[dict[str, object]] = []
    for path in sorted(dist.iterdir()):
        if not path.is_file():
            continue
        if path.name.endswith(".tar.gz"):
            archives.append(
                {
                    "name": path.name,
                    "format": "tar.gz",
                    "size": path.stat().st_size,
                    "sha256": sha256(path),
                    "gzip_mtime": gzip_header_mtime(path),
                    "members": tar_members(path),
                }
            )
        elif path.name.endswith(".zip"):
            archives.append(
                {
                    "name": path.name,
                    "format": "zip",
                    "size": path.stat().st_size,
                    "sha256": sha256(path),
                    "members": zip_members(path),
                }
            )

    metadata_path = dist / "metadata.json"
    metadata: dict[str, object] = {}
    if metadata_path.is_file():
        raw = json.loads(metadata_path.read_text(encoding="utf-8"))
        metadata = {
            "version": raw.get("version"),
            "commit": raw.get("commit"),
            "date": raw.get("date"),
            "tag": raw.get("tag"),
        }

    fingerprint = {
        "schema": "ainovel-w6a-determinism-fingerprint/1",
        "binary_count": len(binaries),
        "archive_count": len(archives),
        "binaries": binaries,
        "archives": archives,
        "goreleaser_metadata": metadata,
    }
    Path(args.output).write_text(
        json.dumps(fingerprint, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    print(f"W6A FINGERPRINT: binaries={len(binaries)} archives={len(archives)}")


if __name__ == "__main__":
    main()
