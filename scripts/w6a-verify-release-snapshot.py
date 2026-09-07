#!/usr/bin/env python3
"""Verify W6A GoReleaser snapshot artifacts without publishing anything."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import tarfile
import tempfile
import zipfile
from pathlib import Path

SCHEMA = "ainovel-w6a-packaging/1"
BINARY = "ainovel-cli"
CHECKSUM_FILE = "ainovel-cli_checksums.txt"
ARCHIVE_RE = re.compile(
    r"^ainovel-cli_(?P<version>.+)_(?P<os>Linux|Darwin|Windows)_"
    r"(?P<arch>x86_64|arm64)\.(?P<ext>tar\.gz|zip)$"
)
EXPECTED_MATRIX = {
    ("Linux", "x86_64", "tar.gz"),
    ("Linux", "arm64", "tar.gz"),
    ("Darwin", "x86_64", "tar.gz"),
    ("Darwin", "arm64", "tar.gz"),
    ("Windows", "x86_64", "zip"),
    ("Windows", "arm64", "zip"),
}


def fail(message: str) -> None:
    raise SystemExit(f"W6A VERIFY FAIL: {message}")


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def normalized_file_names(names: list[str]) -> set[str]:
    result: set[str] = set()
    for name in names:
        clean = name.replace("\\", "/").lstrip("./")
        if clean and not clean.endswith("/"):
            result.add(clean)
    return result


def archive_files(path: Path) -> set[str]:
    if path.name.endswith(".tar.gz"):
        with tarfile.open(path, "r:gz") as tf:
            return normalized_file_names([m.name for m in tf.getmembers() if m.isfile()])
    if path.name.endswith(".zip"):
        with zipfile.ZipFile(path) as zf:
            return normalized_file_names([i.filename for i in zf.infolist() if not i.is_dir()])
    fail(f"unsupported archive format: {path.name}")
    return set()


def parse_checksums(path: Path) -> dict[str, str]:
    if not path.is_file():
        fail(f"missing checksum manifest: {path}")
    result: dict[str, str] = {}
    for raw in path.read_text(encoding="utf-8").splitlines():
        fields = raw.split()
        if len(fields) != 2:
            continue
        digest, name = fields
        name = name.lstrip("*")
        digest = digest.lower()
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            fail(f"invalid SHA-256 entry for {name}")
        if name in result:
            fail(f"duplicate checksum entry: {name}")
        result[name] = digest
    return result


def extract_native_linux_x86(archive: Path, out_dir: Path) -> Path:
    with tarfile.open(archive, "r:gz") as tf:
        members = [m for m in tf.getmembers() if m.isfile() and m.name.replace("\\", "/").lstrip("./") == BINARY]
        if len(members) != 1:
            fail(f"native archive must contain exactly one {BINARY}: {archive.name}")
        src = tf.extractfile(members[0])
        if src is None:
            fail(f"cannot extract native binary from {archive.name}")
        dst = out_dir / BINARY
        dst.write_bytes(src.read())
        dst.chmod(0o755)
        return dst


def verify_version(binary: Path, expected_sha: str, archive_version: str) -> dict[str, str]:
    proc = subprocess.run(
        [str(binary), "--version"],
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        timeout=30,
    )
    if proc.returncode != 0:
        fail(f"packaged native binary --version failed with exit {proc.returncode}")
    lines = [line.strip() for line in proc.stdout.splitlines() if line.strip()]
    if len(lines) < 3 or not lines[0].startswith("ainovel-cli "):
        fail("packaged native binary returned unexpected --version output")
    reported_version = lines[0].split(maxsplit=1)[1]
    fields: dict[str, str] = {"version": reported_version}
    for line in lines[1:]:
        if ":" in line:
            key, value = line.split(":", 1)
            fields[key.strip()] = value.strip()
    if fields.get("commit") != expected_sha:
        fail(f"embedded commit mismatch: got {fields.get('commit')!r}, expected {expected_sha}")
    if fields.get("built") in (None, "", "unknown"):
        fail("embedded build date is missing/unknown")
    if reported_version.lstrip("v") != archive_version.lstrip("v"):
        fail(
            "embedded version does not match archive version: "
            f"{reported_version!r} vs {archive_version!r}"
        )
    return fields


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dist", required=True)
    parser.add_argument("--expected-sha", required=True)
    parser.add_argument("--evidence", required=True)
    args = parser.parse_args()

    dist = Path(args.dist).resolve()
    expected_sha = args.expected_sha.strip().lower()
    if not re.fullmatch(r"[0-9a-f]{40}", expected_sha):
        fail("expected SHA must be a full 40-character lowercase Git SHA")
    if not dist.is_dir():
        fail(f"dist directory does not exist: {dist}")

    archives: list[tuple[Path, str, str, str, str]] = []
    for path in sorted(dist.iterdir()):
        if not path.is_file():
            continue
        match = ARCHIVE_RE.match(path.name)
        if match:
            archives.append(
                (
                    path,
                    match.group("version"),
                    match.group("os"),
                    match.group("arch"),
                    match.group("ext"),
                )
            )

    if len(archives) != 6:
        fail(f"expected exactly 6 platform archives, found {len(archives)}")

    matrix = {(os_name, arch, ext) for _, _, os_name, arch, ext in archives}
    if matrix != EXPECTED_MATRIX:
        fail(f"archive matrix mismatch: {sorted(matrix)}")

    versions = {version for _, version, _, _, _ in archives}
    if len(versions) != 1:
        fail(f"archives do not share one version: {sorted(versions)}")
    archive_version = next(iter(versions))

    checksums = parse_checksums(dist / CHECKSUM_FILE)
    archive_names = {path.name for path, *_ in archives}
    if set(checksums) != archive_names:
        fail(
            "checksum manifest must map one-to-one to platform archives: "
            f"manifest={sorted(checksums)}, archives={sorted(archive_names)}"
        )

    evidence_archives: list[dict[str, str]] = []
    native_archive: Path | None = None
    for path, version, os_name, arch, ext in archives:
        actual = sha256(path)
        expected = checksums[path.name]
        if actual != expected:
            fail(f"checksum mismatch for {path.name}")
        expected_binary = "ainovel-cli.exe" if os_name == "Windows" else BINARY
        expected_files = {expected_binary, "README.md", "LICENSE"}
        files = archive_files(path)
        if files != expected_files:
            fail(
                f"archive contents mismatch for {path.name}: "
                f"got={sorted(files)}, expected={sorted(expected_files)}"
            )
        if os_name == "Linux" and arch == "x86_64":
            native_archive = path
        evidence_archives.append(
            {
                "name": path.name,
                "os": os_name,
                "arch": arch,
                "format": ext,
                "sha256": actual,
            }
        )

    if native_archive is None:
        fail("missing Linux x86_64 archive for native metadata verification")

    with tempfile.TemporaryDirectory(prefix="ainovel-w6a-") as tmp:
        binary = extract_native_linux_x86(native_archive, Path(tmp))
        metadata = verify_version(binary, expected_sha, archive_version)

    evidence = {
        "schema": SCHEMA,
        "candidate_sha": expected_sha,
        "snapshot_version": archive_version,
        "archive_count": len(evidence_archives),
        "archive_matrix": evidence_archives,
        "checksum_manifest": CHECKSUM_FILE,
        "metadata": {
            "version": metadata["version"],
            "commit": metadata["commit"],
            "built": metadata["built"],
        },
        "no_publish": True,
    }
    evidence_path = Path(args.evidence)
    evidence_path.parent.mkdir(parents=True, exist_ok=True)
    evidence_path.write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(
        f"W6A VERIFY PASS: {len(evidence_archives)} archives, "
        f"version={archive_version}, candidate={expected_sha}"
    )


if __name__ == "__main__":
    main()
