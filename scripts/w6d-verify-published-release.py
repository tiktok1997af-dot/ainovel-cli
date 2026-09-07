#!/usr/bin/env python3
import argparse
import hashlib
import json
import re
import tarfile
import zipfile
from pathlib import Path

REPO = "tiktok1997af-dot/ainovel-cli"
BINARY = "ainovel-cli"


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()


def fail(message: str) -> None:
    raise SystemExit(f"W6D published release verification FAIL: {message}")


def archive_names(path: Path):
    if path.suffix == ".zip":
        with zipfile.ZipFile(path) as zf:
            return sorted(x.filename.replace("\\", "/") for x in zf.infolist() if not x.is_dir())
    if path.name.endswith(".tar.gz"):
        with tarfile.open(path, "r:gz") as tf:
            return sorted(x.name.replace("\\", "/") for x in tf.getmembers() if x.isfile())
    fail(f"unsupported release archive type: {path.name}")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--release-json", required=True)
    ap.add_argument("--assets-dir", required=True)
    ap.add_argument("--expected-tag", required=True)
    ap.add_argument("--expected-sha", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    tag = args.expected_tag.strip()
    commit = args.expected_sha.strip().lower()
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        fail(f"tag is not strict stable SemVer: {tag}")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        fail("expected SHA must be a full lowercase Git commit SHA")

    release = json.loads(Path(args.release_json).read_text(encoding="utf-8"))
    if release.get("tag_name") != tag:
        fail(f"release tag mismatch: {release.get('tag_name')!r}")
    if release.get("draft") is not False or release.get("prerelease") is not False:
        fail("first fork release must be a published non-prerelease")
    html_url = str(release.get("html_url") or "")
    if not html_url.startswith(f"https://github.com/{REPO}/releases/tag/{tag}"):
        fail(f"release does not belong to exact fork: {html_url!r}")

    version = tag[1:]
    expected_archives = [
        f"{BINARY}_{version}_Linux_x86_64.tar.gz",
        f"{BINARY}_{version}_Linux_arm64.tar.gz",
        f"{BINARY}_{version}_Darwin_x86_64.tar.gz",
        f"{BINARY}_{version}_Darwin_arm64.tar.gz",
        f"{BINARY}_{version}_Windows_x86_64.zip",
        f"{BINARY}_{version}_Windows_arm64.zip",
    ]
    checksum_name = f"{BINARY}_checksums.txt"
    expected_assets = sorted(expected_archives + [checksum_name])

    assets = release.get("assets") or []
    names = sorted(str(x.get("name") or "") for x in assets)
    if names != expected_assets:
        fail(f"published asset set mismatch: got {names}, expected {expected_assets}")
    if len(set(names)) != len(names):
        fail("duplicate published asset names")

    by_name = {str(x.get("name")): x for x in assets}
    for name in expected_assets:
        want_url = f"https://github.com/{REPO}/releases/download/{tag}/{name}"
        got_url = str(by_name[name].get("browser_download_url") or "")
        if got_url != want_url:
            fail(f"asset {name} has unauthorized download URL: {got_url!r}")

    body = str(release.get("body") or "")
    secret_patterns = [
        r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----",
        r"\bghp_[A-Za-z0-9]{20,}\b",
        r"\bgithub_pat_[A-Za-z0-9_]{20,}\b",
        r"\bAIza[0-9A-Za-z_-]{20,}\b",
        r"(?i)authorization\s*:\s*bearer\s+[A-Za-z0-9._~+/=-]{8,}",
        r"(?i)(?:api[_-]?key|password|secret|token)\s*[:=]\s*[A-Za-z0-9._~+/=-]{8,}",
    ]
    for pattern in secret_patterns:
        if re.search(pattern, body):
            fail("release notes contain secret-like material")

    assets_dir = Path(args.assets_dir)
    for name in expected_assets:
        path = assets_dir / name
        if not path.is_file() or path.stat().st_size <= 0:
            fail(f"downloaded live asset missing/empty: {name}")

    manifest_path = assets_dir / checksum_name
    manifest_entries = {}
    for raw in manifest_path.read_text(encoding="utf-8").splitlines():
        fields = raw.split()
        if len(fields) != 2:
            continue
        digest, name = fields
        name = name.lstrip("*")
        if name not in expected_archives:
            fail(f"checksum manifest references unsupported asset: {name}")
        if name in manifest_entries:
            fail(f"checksum manifest has duplicate entry: {name}")
        digest = digest.lower()
        if not re.fullmatch(r"[0-9a-f]{64}", digest):
            fail(f"invalid SHA-256 for {name}")
        manifest_entries[name] = digest
    if sorted(manifest_entries) != sorted(expected_archives):
        fail("checksum manifest must map one-to-one to all six archives")

    records = []
    for name in expected_archives:
        path = assets_dir / name
        actual = sha256(path)
        if actual != manifest_entries[name]:
            fail(f"published checksum mismatch for {name}")
        exe = "ainovel-cli.exe" if "_Windows_" in name else "ainovel-cli"
        contents = archive_names(path)
        expected_contents = sorted(["LICENSE", "README.md", exe])
        if contents != expected_contents:
            fail(f"archive {name} contents mismatch: {contents}")
        records.append({"name": name, "sha256": actual, "size": path.stat().st_size})

    evidence = {
        "schema": "ainovel-w6d-published-release/1",
        "repository": REPO,
        "tag": tag,
        "release_commit": commit,
        "release_id": release.get("id"),
        "release_url": html_url,
        "draft": False,
        "prerelease": False,
        "asset_count": 7,
        "archive_count": 6,
        "checksum_manifest": checksum_name,
        "checksum_manifest_sha256": sha256(manifest_path),
        "assets": records,
        "asset_urls_exact_fork": True,
        "checksums_verified": True,
        "archive_contents_verified": True,
        "unsupported_artifacts_present": False,
        "release_notes_secret_scan": "PASS",
        "no_upstream_fallback": True,
    }
    Path(args.output).write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(f"W6D published release verification PASS: {tag} / {commit} / 7 assets")


if __name__ == "__main__":
    main()
