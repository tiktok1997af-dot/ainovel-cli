from pathlib import Path


def must_replace(path: str, old: str, new: str, label: str, count: int = 1) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    actual = text.count(old)
    if actual != count:
        raise SystemExit(f"{label}: expected {count} matches, got {actual}")
    p.write_text(text.replace(old, new, count), encoding="utf-8")


# Product updater wiring: transport remains generic, product source is locked.
must_replace(
    "cmd/ainovel-cli/main.go",
    'Repo:           "voocel/ainovel-cli",',
    "Repo:           buildversion.ProductRepository,",
    "main updater repository",
)

# Tests must describe the WEB-only fork, never the upstream release channel.
p = Path("internal/version/update_test.go")
text = p.read_text(encoding="utf-8")
text = text.replace("https://api.github.com/repos/voocel/ainovel-cli/", "https://api.github.com/repos/tiktok1997af-dot/ainovel-cli/")
text = text.replace('releaseURL("voocel/ainovel-cli", target)', "releaseURL(ProductRepository, target)")
marker = "func TestReleaseURL(t *testing.T) {\n"
if text.count(marker) != 1:
    raise SystemExit("update test marker mismatch")
constant_test = '''func TestProductRepository(t *testing.T) {\n\tif ProductRepository != "tiktok1997af-dot/ainovel-cli" {\n\t\tt.Fatalf("ProductRepository = %q", ProductRepository)\n\t}\n}\n\n'''
text = text.replace(marker, constant_test + marker, 1)
if "voocel/ainovel-cli" in text:
    raise SystemExit("update tests still reference upstream release repo")
p.write_text(text, encoding="utf-8")

# Installer examples and release source are fork-only.
p = Path("scripts/install.sh")
text = p.read_text(encoding="utf-8")
count = text.count("voocel/ainovel-cli")
if count != 3:
    raise SystemExit(f"install upstream refs: expected 3, got {count}")
text = text.replace("voocel/ainovel-cli", "tiktok1997af-dot/ainovel-cli")
p.write_text(text, encoding="utf-8")

# GoReleaser publishes to this fork.
must_replace(
    ".goreleaser.yml",
    "release:\n  github:\n    owner: voocel\n    name: ainovel-cli\n",
    "release:\n  github:\n    owner: tiktok1997af-dot\n    name: ainovel-cli\n",
    "goreleaser repository",
)

# Release workflow must never receive AI API credentials.
must_replace(
    ".github/workflows/release.yml",
    '''      - name: Generate release notes\n        run: .github/scripts/gen-changelog.sh > release-notes.md\n        env:\n          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}\n          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}\n          OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}\n''',
    '''      - name: Generate deterministic release notes\n        run: .github/scripts/gen-changelog.sh > release-notes.md\n''',
    "release AI secret block",
)

# Release notes are derived deterministically from Git history only.
Path(".github/scripts/gen-changelog.sh").write_text(r'''#!/bin/sh
# Deterministic release notes from Git history only.
# Usage: .github/scripts/gen-changelog.sh [previous_tag]
#
# This script intentionally performs no AI/API/network calls and consumes no
# AI credentials. Release notes therefore remain reproducible from repository
# history and cannot reintroduce an API-era execution path.
set -eu

PREV_TAG="${1:-$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || true)}"
CURR_TAG="$(git describe --tags --abbrev=0 HEAD 2>/dev/null || printf '%s' HEAD)"

if [ -n "$PREV_TAG" ]; then
    RANGE="${PREV_TAG}..${CURR_TAG}"
    COMMITS="$(git log "$RANGE" --pretty=format:'- %s' --no-merges)"
else
    RANGE="last 50 commits"
    COMMITS="$(git log "$CURR_TAG" --pretty=format:'- %s' --no-merges -50)"
fi

printf '## What changed\n\n'
if [ -n "$COMMITS" ]; then
    printf '%s\n' "$COMMITS"
else
    printf '%s\n' "- No non-merge commits found in ${RANGE}."
fi
''', encoding="utf-8")

# Docker/headless image distribution cannot satisfy visible Chrome + manual login.
for name in (
    ".github/workflows/docker.yml",
    "Dockerfile",
    "docker-compose.yml",
    ".dockerignore",
):
    p = Path(name)
    if not p.exists():
        raise SystemExit(f"expected Docker distribution file missing before removal: {name}")
    p.unlink()

print("W5D D2 supply-chain patch applied")
