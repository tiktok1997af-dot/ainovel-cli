# W6 — PRODUCTION RELEASE & DISTRIBUTION AUTHORITY v0.1

Status: OPEN / AUTHORIZED / NOT LOCKED

Authority base: `main = ce8d452e70888c18d7ee1b696864fa52b9482e42`

W5 is a locked prerequisite. W6 must not reopen W5 behavior unless a new regression is proven.

## Goal

Prove that the locked WEB-only / NO-API product can be distributed from this fork as real release artifacts, installed or updated through supported paths, and launched without content drift or reintroducing any API-era runtime.

The release artifact—not only a source-tree build—becomes the object under test.

## Non-negotiable product boundary

- WEB-only / NO-API remains invariant.
- No OpenAI/Gemini API/Anthropic/OpenRouter/provider HTTP fallback.
- No Ollama/local-inference active product path.
- No upstream repository/update target may replace this fork.
- Release/download/install/update traffic must remain scoped to `tiktok1997af-dot/ainovel-cli` and HTTPS.
- Checksums are mandatory for supported install/update paths.
- The released binary must still use the visible user-authenticated Gemini Web browser path.
- No credential, cookie, token, account identity, raw browser page content, or raw story/session body may be uploaded as CI/release evidence.
- W6 does not weaken W5.5 or W5E authority gates.

## Existing release surface to verify

Current source authority already contains:

- `.github/workflows/release.yml` — tag-triggered release workflow scoped to this fork;
- `.goreleaser.yml` — Linux/macOS/Windows amd64+arm64 build/archive/checksum matrix;
- `scripts/install.sh` — HTTPS + SHA-256 verified Linux/macOS installer;
- `internal/version/update.go` — checksum-verified updater for supported non-Windows in-place update flow;
- README installation/update claims for this fork.

W6 must prove these surfaces work together on real artifacts.

## W6A — Release contract & deterministic packaging gate

Before any official release tag is created:

1. freeze an exact release candidate SHA descended from the W5-on-main authority;
2. run normal Linux/Windows CI and repository-wide W5.5 NO-API audit;
3. run GoReleaser in non-publishing/snapshot mode;
4. verify expected archive matrix and checksum manifest;
5. verify every archive contains exactly one platform binary plus allowed documentation files;
6. verify embedded version/commit metadata points to the exact candidate;
7. verify release notes generation performs no AI/API model call.

Gate: deterministic packaging PASS with no published release.

## W6B — Install/update integrity gate

Using artifacts produced from the exact W6 release candidate:

- Linux installer path must select the exact fork release asset and validate SHA-256 before installation;
- macOS installer logic must remain equivalent where runner availability permits;
- updater tests must prove exact fork targeting, exact asset selection, checksum enforcement, archive validation and safe replacement behavior;
- Windows remains explicit manual archive install/update unless a separately verified safe replacement mechanism is introduced;
- corrupted/missing/ambiguous checksum or wrong asset name must fail closed.

Gate: supported install/update paths PASS without accepting unverified artifacts.

## W6C — Release-artifact WEB-only smoke

The binary under test must come from the W6 release packaging output, not `go build` performed separately for the smoke.

On the real interactive Windows desktop authority runner:

1. unpack the exact Windows release artifact;
2. verify artifact hash against the generated checksum manifest;
3. run the packaged binary with the persistent authenticated Chrome/Gemini Web profile;
4. reach `READY`;
5. execute a bounded production WEB-only request through the real Engine/Workers/local Tools/Store path;
6. verify runtime NO-API marker scan PASS;
7. close cleanly and restore local test configuration;
8. upload sanitized evidence only.

Gate: packaged release binary behaves equivalently to the W5E-authorized source candidate.

## W6D — First fork release publication gate

Only after W6A–C PASS may an official version tag/release be authorized.

Publication must prove:

- tag points to the exact locked W6 release SHA;
- GitHub Release is produced only by this fork's release workflow;
- all expected platform archives and `ainovel-cli_checksums.txt` are present;
- published checksums match downloaded assets;
- no unsupported Docker/API/provider artifact is published;
- release notes are deterministic and contain no secret material;
- a fresh supported install from the published release succeeds;
- published Windows archive can reach the WEB-only runtime readiness gate on the desktop authority environment.

No version tag or release is authorized merely by opening W6.

## W6.5 — Post-release audit (blocking)

After publication, rerun repository/release audit against both source authority and published assets:

- W5.5 repository-wide NO-API audit PASS;
- current `main` remains descended from the exact W6 release authority;
- release assets map one-to-one to checksum entries;
- installer/updater repository identity remains `tiktok1997af-dot/ainovel-cli`;
- README install/update instructions match the published artifacts;
- no credential-bearing or raw Gemini/story evidence is present in release/CI artifacts.

## Lock criteria

W6 may be AUTHOR-CONFIRMED / LOCKED only when all of the following are true on one exact release authority chain:

- W6A packaging gate PASS;
- W6B install/update integrity gate PASS;
- W6C real packaged-binary desktop WEB-only smoke PASS;
- W6D official fork release publication PASS;
- W6.5 post-release audit PASS;
- post-publication CI/NO-API gates PASS;
- provenance records exact candidate SHA, tag, release, workflow runs, artifact names/checksums and sanitized desktop evidence.

Until then:

**W6 = OPEN / AUTHORIZED / NOT LOCKED.**

## First implementation step

`W6A — RELEASE CONTRACT & DETERMINISTIC PACKAGING GATE v0.1`

Start with a snapshot/dry-run packaging verifier. Do not create an official version tag or GitHub Release during W6A.
