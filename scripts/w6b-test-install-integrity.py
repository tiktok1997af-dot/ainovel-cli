#!/usr/bin/env python3
"""W6B installer integrity harness using exact GoReleaser snapshot artifacts.

The harness never accesses the network. It places a deterministic fake `curl`
in PATH which only serves the exact fork release/API URLs expected by install.sh.
Evidence contains only release provenance and PASS/FAIL scenario metadata.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import tarfile
import tempfile
from pathlib import Path

REPO = "tiktok1997af-dot/ainovel-cli"
BINARY = "ainovel-cli"


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def read_archive_binary(path: Path) -> bytes:
    with tarfile.open(path, "r:gz") as tf:
        matches = [m for m in tf.getmembers() if m.isfile() and m.name == BINARY]
        if len(matches) != 1:
            raise RuntimeError(f"{path.name}: expected one root binary, found {len(matches)}")
        fh = tf.extractfile(matches[0])
        if fh is None:
            raise RuntimeError(f"{path.name}: cannot extract binary")
        return fh.read()


def make_fake_tools(root: Path) -> Path:
    fakebin = root / "fakebin"
    fakebin.mkdir()
    uname = fakebin / "uname"
    uname.write_text(
        "#!/bin/sh\n"
        "case \"$1\" in\n"
        "  -s) printf '%s\\n' \"$W6B_UNAME_S\" ;;\n"
        "  -m) printf '%s\\n' \"$W6B_UNAME_M\" ;;\n"
        "  *) exit 2 ;;\n"
        "esac\n",
        encoding="utf-8",
    )
    uname.chmod(0o755)

    curl = fakebin / "curl"
    curl.write_text(
        "#!/usr/bin/env python3\n"
        "import json, os, shutil, sys\n"
        "from pathlib import Path\n"
        "args = sys.argv[1:]\n"
        "out = None\n"
        "url = None\n"
        "i = 0\n"
        "while i < len(args):\n"
        "    if args[i] == '-o':\n"
        "        i += 1\n"
        "        out = args[i]\n"
        "    elif args[i].startswith('https://'):\n"
        "        url = args[i]\n"
        "    i += 1\n"
        "if not url:\n"
        "    raise SystemExit('fake curl: missing URL')\n"
        "log = Path(os.environ['W6B_URL_LOG'])\n"
        "with log.open('a', encoding='utf-8') as fh:\n"
        "    fh.write(url + '\\n')\n"
        "repo = os.environ['W6B_REPO']\n"
        "version = os.environ['W6B_VERSION']\n"
        "tag = 'v' + version\n"
        "mode = os.environ['W6B_MODE']\n"
        "dist = Path(os.environ['W6B_DIST'])\n"
        "expected_asset = os.environ['W6B_EXPECTED_ASSET']\n"
        "api_latest = f'https://api.github.com/repos/{repo}/releases/latest'\n"
        "api_tag = f'https://api.github.com/repos/{repo}/releases/tags/{tag}'\n"
        "base = f'https://github.com/{repo}/releases/download/{tag}/'\n"
        "if url in (api_latest, api_tag):\n"
        "    sys.stdout.write(json.dumps({'tag_name': tag}) + '\\n')\n"
        "    raise SystemExit(0)\n"
        "if not url.startswith(base):\n"
        "    raise SystemExit(f'fake curl: unauthorized URL {url}')\n"
        "name = url[len(base):]\n"
        "if '/' in name or not name:\n"
        "    raise SystemExit('fake curl: invalid asset path')\n"
        "src = dist / name\n"
        "if name == 'ainovel-cli_checksums.txt':\n"
        "    text = src.read_text(encoding='utf-8')\n"
        "    lines = text.splitlines()\n"
        "    if mode == 'bad_checksum':\n"
        "        lines = [('0' * 64 + '  ' + expected_asset) if line.split()[-1].lstrip('*') == expected_asset else line for line in lines if line.split()]\n"
        "    elif mode == 'missing_checksum':\n"
        "        lines = [line for line in lines if not line.split() or line.split()[-1].lstrip('*') != expected_asset]\n"
        "    elif mode == 'duplicate_checksum':\n"
        "        match = [line for line in lines if line.split() and line.split()[-1].lstrip('*') == expected_asset]\n"
        "        if len(match) != 1:\n"
        "            raise SystemExit('fake curl: fixture missing checksum entry')\n"
        "        lines.append(match[0])\n"
        "    payload = ('\\n'.join(lines) + '\\n').encode()\n"
        "    if out:\n"
        "        Path(out).write_bytes(payload)\n"
        "    else:\n"
        "        sys.stdout.buffer.write(payload)\n"
        "    raise SystemExit(0)\n"
        "if name != expected_asset or not src.is_file():\n"
        "    raise SystemExit(f'fake curl: unexpected asset {name}')\n"
        "if not out:\n"
        "    sys.stdout.buffer.write(src.read_bytes())\n"
        "else:\n"
        "    shutil.copyfile(src, out)\n",
        encoding="utf-8",
    )
    curl.chmod(0o755)
    return fakebin


def expected_urls(version: str, asset: str, latest: bool) -> set[str]:
    tag = "v" + version
    api = (
        f"https://api.github.com/repos/{REPO}/releases/latest"
        if latest
        else f"https://api.github.com/repos/{REPO}/releases/tags/{tag}"
    )
    base = f"https://github.com/{REPO}/releases/download/{tag}"
    return {api, f"{base}/{asset}", f"{base}/{BINARY}_checksums.txt"}


def run_scenario(
    *,
    root: Path,
    fakebin: Path,
    installer: Path,
    dist: Path,
    version: str,
    name: str,
    uname_s: str,
    uname_m: str,
    os_name: str | None,
    arch_name: str | None,
    latest: bool,
    mode: str,
    should_succeed: bool,
    expect_no_transport: bool = False,
) -> dict[str, object]:
    scenario_root = root / name
    install_dir = scenario_root / "install"
    install_dir.mkdir(parents=True)
    log = scenario_root / "urls.txt"
    asset = ""
    archive: Path | None = None
    if os_name is not None and arch_name is not None:
        asset = f"{BINARY}_{version}_{os_name}_{arch_name}.tar.gz"
        archive = dist / asset
        if not archive.is_file():
            raise RuntimeError(f"missing snapshot archive for scenario {name}: {asset}")

    env = os.environ.copy()
    env.update(
        {
            "PATH": str(fakebin) + os.pathsep + env.get("PATH", ""),
            "AINOVEL_INSTALL_DIR": str(install_dir),
            "AINOVEL_VERSION": "latest" if latest else "v" + version,
            "W6B_UNAME_S": uname_s,
            "W6B_UNAME_M": uname_m,
            "W6B_URL_LOG": str(log),
            "W6B_REPO": REPO,
            "W6B_VERSION": version,
            "W6B_MODE": mode,
            "W6B_DIST": str(dist),
            "W6B_EXPECTED_ASSET": asset,
        }
    )
    proc = subprocess.run(
        ["sh", str(installer)],
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        timeout=60,
        check=False,
    )
    success = proc.returncode == 0
    if success != should_succeed:
        raise RuntimeError(
            f"scenario {name}: exit={proc.returncode}, expected_success={should_succeed}\n{proc.stdout[-2000:]}"
        )

    installed = install_dir / BINARY
    if should_succeed:
        if archive is None or not installed.is_file():
            raise RuntimeError(f"scenario {name}: successful install did not create binary")
        expected_binary = read_archive_binary(archive)
        if installed.read_bytes() != expected_binary:
            raise RuntimeError(f"scenario {name}: installed binary differs from exact snapshot archive")
    elif installed.exists():
        raise RuntimeError(f"scenario {name}: failed integrity case wrote an installed binary")

    urls = log.read_text(encoding="utf-8").splitlines() if log.exists() else []
    if expect_no_transport:
        if urls:
            raise RuntimeError(f"scenario {name}: unsupported platform unexpectedly used transport: {urls}")
    else:
        allowed = expected_urls(version, asset, latest)
        if set(urls) != allowed or len(urls) != 3:
            raise RuntimeError(f"scenario {name}: URL provenance mismatch: {urls}")
        for url in urls:
            if "kentjuno" in url or "voocel/ainovel-cli" in url or not url.startswith("https://"):
                raise RuntimeError(f"scenario {name}: forbidden source URL {url}")

    return {
        "name": name,
        "status": "PASS",
        "expected_success": should_succeed,
        "transport_requests": len(urls),
        "installed_binary_sha256": sha256_bytes(installed.read_bytes()) if should_succeed else None,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dist", required=True)
    parser.add_argument("--installer", required=True)
    parser.add_argument("--expected-sha", required=True)
    parser.add_argument("--evidence", required=True)
    args = parser.parse_args()

    dist = Path(args.dist).resolve()
    installer = Path(args.installer).resolve()
    metadata = json.loads((dist / "metadata.json").read_text(encoding="utf-8"))
    if metadata.get("commit") != args.expected_sha:
        raise SystemExit(f"snapshot commit mismatch: {metadata.get('commit')} != {args.expected_sha}")
    version = str(metadata.get("version", ""))
    if not version or version.startswith("v"):
        raise SystemExit(f"unexpected snapshot version: {version!r}")

    matrix = [
        ("Linux", "x86_64", "Linux", "x86_64"),
        ("Linux", "aarch64", "Linux", "arm64"),
        ("Darwin", "x86_64", "Darwin", "x86_64"),
        ("Darwin", "arm64", "Darwin", "arm64"),
    ]
    for _, _, os_name, arch_name in matrix:
        archive = dist / f"{BINARY}_{version}_{os_name}_{arch_name}.tar.gz"
        if not archive.is_file():
            raise SystemExit(f"missing matrix archive: {archive.name}")

    with tempfile.TemporaryDirectory(prefix="ainovel-w6b-") as tmp:
        root = Path(tmp)
        fakebin = make_fake_tools(root)
        scenarios: list[dict[str, object]] = []
        for uname_s, uname_m, os_name, arch_name in matrix:
            scenarios.append(
                run_scenario(
                    root=root,
                    fakebin=fakebin,
                    installer=installer,
                    dist=dist,
                    version=version,
                    name=f"pinned-{os_name.lower()}-{arch_name}",
                    uname_s=uname_s,
                    uname_m=uname_m,
                    os_name=os_name,
                    arch_name=arch_name,
                    latest=False,
                    mode="good",
                    should_succeed=True,
                )
            )
        scenarios.append(
            run_scenario(
                root=root,
                fakebin=fakebin,
                installer=installer,
                dist=dist,
                version=version,
                name="latest-linux-x86_64",
                uname_s="Linux",
                uname_m="x86_64",
                os_name="Linux",
                arch_name="x86_64",
                latest=True,
                mode="good",
                should_succeed=True,
            )
        )
        for mode in ("bad_checksum", "missing_checksum", "duplicate_checksum"):
            scenarios.append(
                run_scenario(
                    root=root,
                    fakebin=fakebin,
                    installer=installer,
                    dist=dist,
                    version=version,
                    name=mode.replace("_", "-"),
                    uname_s="Linux",
                    uname_m="x86_64",
                    os_name="Linux",
                    arch_name="x86_64",
                    latest=False,
                    mode=mode,
                    should_succeed=False,
                )
            )
        scenarios.append(
            run_scenario(
                root=root,
                fakebin=fakebin,
                installer=installer,
                dist=dist,
                version=version,
                name="unsupported-arch",
                uname_s="Linux",
                uname_m="mips64",
                os_name=None,
                arch_name=None,
                latest=False,
                mode="good",
                should_succeed=False,
                expect_no_transport=True,
            )
        )
        scenarios.append(
            run_scenario(
                root=root,
                fakebin=fakebin,
                installer=installer,
                dist=dist,
                version=version,
                name="unsupported-os",
                uname_s="FreeBSD",
                uname_m="x86_64",
                os_name=None,
                arch_name=None,
                latest=False,
                mode="good",
                should_succeed=False,
                expect_no_transport=True,
            )
        )

    evidence = {
        "schema": "ainovel-w6b-installer-integrity/1",
        "candidate_sha": args.expected_sha,
        "product_repository": REPO,
        "snapshot_version": version,
        "scenario_count": len(scenarios),
        "scenarios": scenarios,
        "fork_url_lock": "PASS",
        "checksum_fail_closed": "PASS",
        "platform_arch_mapping": "PASS",
        "network_access": "FAKE_TRANSPORT_ONLY",
    }
    Path(args.evidence).write_text(
        json.dumps(evidence, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )
    print(f"W6B INSTALLER INTEGRITY PASS: {len(scenarios)} scenarios, candidate={args.expected_sha}")


if __name__ == "__main__":
    main()
