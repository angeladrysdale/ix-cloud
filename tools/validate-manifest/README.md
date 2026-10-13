# validate-manifest

Validates a container's manifest file against the **built image** — proving the
manifest accurately describes what the image is and does, before you push (pushing a
compliant CalVer tag auto-deploys to production, so this is the last cheap check).

It is the same tool vendored at `examples/rtd-simple/scripts/validate-manifest.py`,
where the sample's `make release` gate runs it. Keep the two copies in sync.

## What it checks

- **Manifest structure** — valid JSON; CalVer `metadata.version`; each `rpc` in
  `package.Service/Method` form; lifecycle weights sum to 1.0; metrics/health
  declared; egress policy present.
- **Image inspect** — architecture `amd64`, os `linux`, non-root user; the
  `agent-manifest-version` label is present; and the core check: the embedded
  `agent-manifest` label is valid JSON and **deep-equals** the manifest file (with a
  section-level diff if not).
- **Live endpoints** (runs the container on random host ports) — gRPC port open, the
  declared service is served (reflection), gRPC health returns `SERVING`, `/metrics`
  returns 200, every declared metric is exposed, and `rpc_server_duration` has the
  exact seconds buckets (`0.001 … 0.010`, `+Inf`) and a tag set matching the declared
  `fields`.

## Requirements

- `docker` and `python3` (standard library only).
- `grpcurl` is optional. If it is not installed, the script auto-falls back to a
  dockerized `fullstorydev/grpcurl` to introspect the gRPC endpoints; without either,
  those specific checks are skipped (not failed).

## Usage

```bash
python3 validate-manifest.py <image> [--manifest PATH] [--models PATH] [--static-only]

# examples
python3 validate-manifest.py my-rtd:dev --manifest ./manifest/manifest.json --models ./models
python3 validate-manifest.py my-rtd:dev --static-only   # structure + image inspect only, no run
```

- `<image>` — the image ref to validate (default `rtd-simple:local`).
- `--manifest` — manifest file to validate (default `manifest/manifest.json`). Only
  the public manifest is read; an internal `global-config.json` is never read.
- `--models` — directory mounted read-only at `/models` so a model-presence check
  sees a delivered model (default `models`).
- `--static-only` — skip running the container (manifest + image inspect only).

## Exit codes and output

Each check prints `[PASS] / [FAIL] / [WARN] / [SKIP]`, ending with a summary line.
**Exit 0** when every check passes, **exit 1** if any check FAILs (WARN and SKIP do
not fail the run) — so it works as a release/CI gate.

```
== Image inspect: my-rtd:dev ==
  [PASS] architecture is amd64 (required: amd64)
  [PASS] runs as non-root user: 1000:1000
  [PASS] embedded agent-manifest matches manifest file exactly
...
Summary: 36 pass, 0 fail, 1 warn, 0 skip
```

One expected `WARN`: `GetMutations` is often not enumerable via gRPC reflection
because generated ARTF stubs don't register their proto descriptor globally. The
service is still served and callable by a compiled client (how Index calls it), so
this is a tooling limitation, not a contract failure — validate `GetMutations`
behavior with a compiled client/harness.
