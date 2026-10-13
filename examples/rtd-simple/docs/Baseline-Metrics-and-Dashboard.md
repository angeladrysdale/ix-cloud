# Baseline metrics (CPU, memory, response time, QPS) and dashboard

How to expose the four baseline signals from any Index Cloud RTD container, and
build a Grafana dashboard for them. rtd-simple is the worked example; the recipe
is generic (swap the `rtd_simple_` metric prefix for your own).

Two of the four come for free from the metric Index already requires. The other
two need a small, dependency-free addition.

| Signal | Source |
|--------|--------|
| QPS | Derived from the required `rpc_server_duration` histogram |
| Response time | Derived from the required `rpc_server_duration` histogram |
| CPU | Self-reported from the container cgroup |
| Memory | Self-reported from the container cgroup |

## Part 1: Metrics in the container

### QPS and response time (already required)

Every Index Cloud container must expose the `rpc_server_duration` histogram on its
HTTP `/metrics` endpoint: handler duration in seconds, exact buckets 0.001, 0.002,
0.003, 0.004, 0.005, 0.007, 0.010, with the fixed tags `rpc_system`,
`rpc_grpc_status_code`, `rpc_method`, `rpc_service`, `error_type`. Record each RPC
into it from a server interceptor. If you have that, QPS and response time are
just PromQL (Part 2); no new instrumentation is needed. Do not add extra tags to
this metric, and keep total metric cardinality under 10,000.

### CPU and memory (add a cgroup reader)

Read the container's own usage and limits from the kernel cgroup virtual files at
scrape time. This needs no dependency and no background thread, and it is safe on
a scratch base image and a read-only root filesystem, because the cgroup files are
a kernel virtual filesystem readable by the non-root runtime user.

Files to read (cgroup v2 first, with a v1 fallback):

| Value | cgroup v2 | cgroup v1 |
|-------|-----------|-----------|
| CPU used (seconds) | `/sys/fs/cgroup/cpu.stat` line `usage_usec N` (N / 1e6) | `/sys/fs/cgroup/cpuacct/cpuacct.usage` (ns / 1e9) |
| CPU limit (cores) | `/sys/fs/cgroup/cpu.max` = `quota period` (cores = quota/period; `max` = unlimited) | `cpu.cfs_quota_us` / `cpu.cfs_period_us` |
| Memory used (bytes) | `/sys/fs/cgroup/memory.current` | `/sys/fs/cgroup/memory/memory.usage_in_bytes` |
| Memory limit (bytes) | `/sys/fs/cgroup/memory.max` (bytes or `max`) | `memory.limit_in_bytes` (huge value = unlimited) |

When a limit is unlimited, fall back to a sensible denominator: the CPUs visible
to the process for CPU, and `/proc/meminfo` `MemTotal` for memory.

Expose these series (use your own prefix in place of `rtd_simple_`):

| Metric | Type | Value |
|--------|------|-------|
| `rtd_simple_cpu_seconds_total` | counter | cumulative CPU seconds |
| `rtd_simple_cpu_limit_cores` | gauge | CPU allocation in cores (the CPU % denominator) |
| `rtd_simple_memory_bytes` | gauge | current memory in bytes |
| `rtd_simple_memory_limit_bytes` | gauge | memory allocation in bytes (the memory % denominator) |
| `rtd_simple_memory_percent` | gauge | `memory_bytes / memory_limit_bytes * 100` |

Optional convenience: a single-number latency gauge `rtd_simple_response_seconds_avg`
computed as the running total of handler durations divided by the request count.
Percentiles still come from the `rpc_server_duration` histogram.

The percentages are "percent of allocation." Index sets your pod's CPU and memory
limits from your per-replica manifest, so those limits are the denominators. Index
also collects pod CPU and memory separately via cAdvisor; the self-reported series
are for visibility in your own endpoint alongside the app signals.

Go reference: see `internal/sysstats/sysstats.go` (the cgroup reader) and
`internal/metrics/metrics.go` (the exporter) in this project. The reader parses the
files above and returns floats; the exporter reads them fresh inside the `/metrics`
handler and prints the series.

Other languages: instead of a custom reader you can register your Prometheus
client's standard process and runtime collectors, which give `process_cpu_seconds_total`
and `process_resident_memory_bytes` for free. That is less code but adds the client
library (larger binary, more to vulnerability-scan), and you still read the cgroup
for the limits, since the process collectors do not report them.

## Part 2: The dashboard

### Pick the right data source first

Use the Prometheus data source that scrapes your container (labeled "Metrics" in
this Grafana). Do not use the Loki "Logs" source. If a query fails with
`parse error ... expecting NUMBER or { or (`, you are on the Logs data source:
switch the data-source picker to Metrics and rerun.

### Quick look: Explore

1. Left nav, Explore.
2. Data source: Metrics.
3. Query editor in Code mode (not Builder).
4. Paste one expression from the table below and run it.

### Persistent: a dashboard

Fastest is to import the ready-made dashboard from the repository's
`monitoring/rtd-simple/` folder, `rtd-simple-baseline-dashboard.json`:

1. Dashboards, New, Import.
2. Upload the JSON (or paste it).
3. When prompted for the "Metrics (Prometheus)" data source, select your
   Prometheus source, then Import.

To build it by hand instead: Dashboards, New, New dashboard, Add visualization;
choose the Metrics data source; Code mode; paste one expression; set the Unit
(Standard options); title the panel; Apply; repeat; Save.

### The queries and units

Each is a single expression: paste exactly one, with no comment line. The `sum` and
`avg` wrappers collapse all replicas into one number; drop them to see per-instance
lines.

| Panel | Expression | Unit |
|-------|-----------|------|
| CPU % | `100 * sum(rate(rtd_simple_cpu_seconds_total[5m])) / sum(rtd_simple_cpu_limit_cores)` | Percent (0-100) |
| Memory % | `avg(rtd_simple_memory_percent)` | Percent (0-100) |
| QPS | `sum(rate(rpc_server_duration_count{rpc_method="GetMutations"}[1m]))` | requests/sec (reqps) |
| Response time, avg | `sum(rate(rpc_server_duration_sum[5m])) / sum(rate(rpc_server_duration_count[5m]))` | seconds (s) |
| Response time, p95 | `histogram_quantile(0.95, sum by (le) (rate(rpc_server_duration_bucket[5m])))` | seconds (s) |

## Part 3: The peak overview (sizing QPS to the worst pod)

The baseline dashboard above reports averages, which answer "what is typical."
For capacity planning the question is different: "how hard can I push before the
*worst* pod or the *worst-case* latency breaks the SLA." A fleet's safe capacity
is set by its hottest pod and its highest-percentile latency, the lowest common
denominator, not by a comfortable average that hides a saturating replica.

Import `rtd-simple-performance-dashboard.json` from the repository's
`monitoring/rtd-simple/` folder (same import steps as above). It mirrors the baseline
signals but with peak aggregations.

### How to read it

- **Peak CPU %** is `max()` across pods, reduced by `max` over the time range. It
  is the true ceiling. If the average reads 70% but four pods sit at 84%, this
  tile shows 84%, which is the number that matters.
- **Peak memory %** is the same, for memory.
- **Peak response time p99** comes from the `rpc_server_duration` histogram, in
  milliseconds, flagged red at the 5 ms SLA. p95 and p99 are also trended over
  time with the 5 ms line.
- **Safe fleet QPS** is the one actionable capacity number:
  `sum(QPS) * $headroom / peak_CPU%`, reduced by `min` over the range. It is the
  total QPS at which the hottest pod would sit at the chosen headroom. Lower the
  headroom dropdown to size more conservatively.
- **Average CPU % per pod** plots the mean per-pod CPU (solid) with a shaded band
  between the hottest and coolest pod (max and min across pods), plus the 85% SLA
  headroom line. The band is the fleet imbalance; size to the band's upper edge
  (the peak), not the average.
- **Average QPS per pod** plots the mean per-pod QPS (solid) with a shaded band
  between the busiest and quietest pod (max and min across pods). The band is the
  load spread; a wide band means traffic is uneven across replicas. This stays
  readable at any pod count, unlike a line-per-pod chart.

### The headroom variable

A dashboard variable `$headroom` (dropdown, default 70%) feeds the Safe fleet QPS
estimate. It is the target peak CPU you want the hottest pod to stay under:
drop it to 60% for a conservative ceiling, raise it to 80% if you are willing to
run hotter. The Safe QPS tile recomputes live as you change it.

### Peak-overview gotchas

- **Histogram ceiling.** The required buckets top out at 0.010 s, so any true p99
  above 10 ms displays as about 10 ms. Fine for a light container (sub-millisecond
  handler time); a heavier container whose real peak exceeds 10 ms will have it
  clipped. The bucket layout is fixed by the Index metric contract, so raising the
  ceiling is an Index conversation, not a container change.

## Gotchas

- Wrong data source (Logs instead of Metrics) is the usual cause of a PromQL parse
  error.
- A metric only appears after a deployed image version that emits it is scraped.
  CPU % and memory % need the version that exports the limits; QPS and response
  time work on any version that exposes `rpc_server_duration`.
- If a panel shows "no data," widen the time range so the `[5m]` or `[1m]` window
  contains at least two scrapes.
- Paste one expression per query box, not the whole block.
