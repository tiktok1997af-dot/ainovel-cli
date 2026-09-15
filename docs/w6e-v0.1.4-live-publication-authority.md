# W6E — v0.1.4 LIVE PUBLICATION AUTHORITY

Status: OPEN / AUTHOR-AUTHORIZED / NOT LOCKED

## Frozen product authority

The product-development authority remains frozen at:

`95ffc80a90f8bf4a014c70ea7cc8bf47c51df31e`

No product-runtime change is authorized by this release gate. The publication-control diff is limited to the W6E v0.1.4 prepublication workflow, live publisher workflow, and this authority document.

## Monotonic release authority

Existing live release history is preserved:

- previous live release: `v0.1.3`
- target live release: `v0.1.4`

The target tag and GitHub Release must be absent before first publication. Existing tags/releases `v0.1.0` through `v0.1.3` must never be rewritten or replaced.

## One-shot publication authorization

The only production publication branch is:

`w6e-v0.1.4-publication-authority`

The live publisher may proceed only when that branch is fast-forwarded to the exact prepublication-authority candidate whose HEAD commit message is exactly:

`release(w6e): authorize v0.1.4 from frozen 95ffc80a`

Before publication, the fail-closed prepublication gate must PASS on the same control candidate and must prove:

- the frozen SHA is an ancestor;
- only the three W6E publication-control files differ from frozen product authority;
- `v0.1.3` tag and live release exist;
- `v0.1.4` tag and live release are absent;
- frozen release/Goreleaser contracts remain fork-owned and SHA-256 protected;
- repository-wide WEB-only / NO-API and Go regressions PASS on the exact frozen product SHA;
- release notes are deterministic and pass secret-like material scanning;
- the exact frozen SHA produces a valid non-publishing GoReleaser snapshot.

## Live publication assertions

Publication is accepted only if all of the following PASS:

- tag `v0.1.4` points exactly to frozen product SHA `95ffc80a90f8bf4a014c70ea7cc8bf47c51df31e`;
- GitHub Release `v0.1.4` is live, non-draft, and non-prerelease;
- release assets and SHA-256 checksum manifest pass independent verification;
- pinned `v0.1.4` install and `latest` install both resolve to the exact frozen product SHA;
- no upstream/API fallback artifact is published;
- live Windows x86_64 release asset passes the real WEB-only desktop readiness/restart authority using the authenticated persistent browser profile;
- transient runtime/config state is cleaned and the local config is restored exactly;
- only sanitized evidence is retained.

## Boundary

Development remains `100% / FROZEN` independently of publication status. This gate does not merge `main` and does not authorize W6.5/post-release work automatically.
