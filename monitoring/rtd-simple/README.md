# rtd-simple Grafana dashboards

Ready-to-import Grafana dashboards for the metrics this container exposes on its HTTP
`/metrics` endpoint. All use a Prometheus data source (labelled "Metrics"), not Loki
("Logs").

| File | What it shows |
|------|---------------|
| `rtd-simple-performance-dashboard.json` | **Peak-oriented overview.** Worst-pod CPU %, memory %, p99 latency, a "safe fleet QPS" capacity estimate, and average-with-min/max bands for CPU and QPS. Size QPS to the hottest pod, not the average. |
| `rtd-simple-baseline-dashboard.json` | The four baseline signals aggregated across pods: CPU %, memory %, QPS, response time (avg + p95). |
| `rtd-simple-cpu-distribution-dashboard.json` | Per-pod CPU % distribution (time series, sorted bar gauge, spread/stddev, heatmap) to spot pods running hotter than others. |
| `rtd-simple-qps-vs-cpu-dashboard.json` | Correlates QPS vs CPU per pod (scatter + CPU-per-request normalizer) to tell traffic-driven load apart from efficiency differences. |

## Importing into Grafana

1. In Grafana, go to **Dashboards -> New -> Import**.
2. Click **Upload dashboard JSON file** and choose one of the files above (or paste
   its contents).
3. When prompted for the **Metrics (Prometheus)** data source, select the Prometheus
   data source that scrapes this container's `/metrics` endpoint, then **Import**.
4. Save it into a folder you own.

Notes:

- Use the **Prometheus** ("Metrics") data source, not Loki ("Logs"). A PromQL
  `parse error ... expecting NUMBER or { or (` means you picked the Logs source.
- The JSON has no fixed UID, so each import creates a new copy. Delete a prior copy
  before re-importing if you do not want duplicates.
- A metric only appears after a deployed image version that emits it has been
  scraped; widen the time range if a panel shows "no data" so the rate window
  contains at least two scrapes.

See
[`examples/rtd-simple/docs/Baseline-Metrics-and-Dashboard.md`](../../examples/rtd-simple/docs/Baseline-Metrics-and-Dashboard.md)
for how the underlying metrics are produced and the PromQL behind each panel, and
[`examples/rtd-simple/`](../../examples/rtd-simple/) for the container that emits
them.

## Reuse

These dashboards are part of the rtd-simple sample and are licensed **Apache-2.0** (see
[`examples/rtd-simple/LICENSE`](../../examples/rtd-simple/LICENSE)): use, modify, and
redistribute them under the Apache License, Version 2.0.
