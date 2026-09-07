# W6D — FIRST FORK RELEASE PUBLICATION GATE v0.1

Status: OPEN / AUTHORIZED / NOT LOCKED

Prerequisites:
- W6A = LOCKED
- W6B = LOCKED
- W6C = LOCKED
- exact W6C authority ancestor = `c3a08d1069345900e0fc5bbb39cfea5e732c3924`

## First release version authority

The fork currently has no Git tag and no GitHub Release. The first official fork release is therefore frozen as:

`v0.1.0`

This is a pre-1.0 semantic version. It does not claim W6/W6.5 or the whole product roadmap is complete.

## Publication authority

The W6D publication workflow MUST fail closed unless all of the following are true before any remote tag is created:

1. repository identity is exactly `tiktok1997af-dot/ainovel-cli`;
2. execution is on the authorized W6 branch and the one-shot authorization commit message matches exactly;
3. the exact publication SHA descends from the locked W6C authority SHA;
4. the diff since W6C is confined to W6D release-authority files and contains no WebAI/Engine/Workers/Tools/Store product-runtime change;
5. no remote tag exists and no GitHub Release exists yet;
6. release tag is exactly `v0.1.0` and passes strict stable SemVer syntax;
7. normal Go tests, W5.5 NO-API audit, W6B updater integrity tests, deterministic release-note generation and a non-publishing GoReleaser snapshot all pass;
8. release workflow remains locked to this fork, HTTPS assets, SHA-256 checksums and the six Linux/Darwin/Windows amd64/arm64 archives.

Only after those preconditions pass may the workflow create a lightweight `v0.1.0` tag pointing directly to the exact W6D authority SHA and invoke GoReleaser publication with the repository-scoped `GITHUB_TOKEN`.

## One-shot authorization sentinel

This document revision is the final W6D publication candidate sentinel. Publication remains inert while this commit is only on `w6d-first-fork-release-publication`. After the exact SHA passes the non-publishing prepublication and publisher dry-contract gates, the W6 authority branch may be fast-forwarded to this same SHA. Only that exact branch transition with this commit message authorizes the one-shot `v0.1.0` publication workflow.

## Published release assertions

The live release must prove:
- repository = `tiktok1997af-dot/ainovel-cli`;
- tag = `v0.1.0`;
- tag ref resolves directly to the exact W6D authority commit;
- release is neither draft nor prerelease;
- exact assets = six platform archives plus `ainovel-cli_checksums.txt`;
- every checksum entry is unique SHA-256 and matches the downloaded live asset;
- every archive contains only `LICENSE`, `README.md` and exactly one root production executable;
- no Docker/provider/API/upstream artifact is published;
- deterministic release notes contain no credential/secret material;
- a fresh Linux install from the published release succeeds with checksum verification;
- installer/updater exact-fork compatibility gates remain PASS;
- the published Windows x86_64 archive, downloaded from the live Release, passes the real Windows Gemini Web readiness/restart authority using the persistent authenticated desktop profile;
- only sanitized evidence is uploaded.

## Publication boundary

W6D completion does not merge PR #14 or `main`. W6 remains OPEN until W6.5 post-release audit completes.
