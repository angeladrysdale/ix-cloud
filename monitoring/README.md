# Monitoring

Grafana dashboards and other observability artifacts for Index Cloud containers.
Each container (or dashboard set) lives in its own subfolder.

| Folder | What it is |
|--------|------------|
| [`rtd-simple/`](rtd-simple/) | Dashboards for the `rtd-simple` sample: a peak-oriented performance dashboard, a baseline (CPU/memory/QPS/latency) dashboard, a per-pod CPU distribution dashboard, and a QPS-vs-CPU correlation dashboard. |

All dashboards target a Prometheus data source (labelled "Metrics" in Grafana), not
Loki ("Logs"), and build on the metrics every Index Cloud container exposes on its
HTTP `/metrics` endpoint (the required `rpc_server_duration` histogram, plus the
self-reported CPU/memory/latency gauges the examples add). See each subfolder's
`README.md` for what the panels show and step-by-step import instructions.

## Adding dashboards

Put a new container's dashboards in their own subfolder with a `README.md` that lists
each dashboard and the import steps. Keep dashboards free of fixed UIDs so importing
never collides with a provisioned dashboard, and keep queries data-source-agnostic
(use the `${DS_METRICS}` input) so they import cleanly into any Grafana.
