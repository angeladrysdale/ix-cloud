# Tools

Standalone tooling for building and validating Index Cloud containers. Each tool
lives in its own folder with its own `README.md`.

| Tool | What it does |
|------|--------------|
| [`validate-manifest/`](validate-manifest/) | Validates a container's `manifest.json` **against the built image**: confirms the image carries the manifest as the `agent-manifest` label (and that it matches the file), then runs the container and checks the declared gRPC/HTTP endpoints and the required `rpc_server_duration` metric contract live. Exit 0 = all pass, 1 = any failure, so it drops straight into a release gate or CI. |

## Adding a tool

Put it in its own subfolder with a `README.md` describing what it does, its
requirements, how to run it, and its exit/output contract. Prefer tools that are
dependency-light and work against any Index Cloud container, not just one example.
