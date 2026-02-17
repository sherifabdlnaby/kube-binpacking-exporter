# CLAUDE.md

## Project Overview

Prometheus exporter that monitors Kubernetes cluster binpacking efficiency. Compares pod resource requests against node allocatable capacity using informer-based caching (zero API calls per scrape).

**Purpose**: Helps identify scheduling inefficiency by showing how well pods are packed onto nodes. High allocatable with low allocated means wasted capacity. High allocated means good utilization.

## Architecture

- **Flat layout**: 3 Go files in `package main` at root — no `pkg/`, `internal/`, `cmd/`
- **Plain client-go + informers**: No controller-runtime. This is an exporter, not a controller
- **Resource-agnostic**: Metrics use a `resource` label. Adding GPU/ephemeral-storage is config-only (`--resources`)
- **Scrape-time computation**: `MustNewConstMetric` in `Collect()` — avoids stale metrics for removed nodes
- **Init container aware**: Correctly accounts for init containers using Kubernetes scheduler semantics

## File Map

| File | Role | Key Functions |
|------|------|---------------|
| `main.go` | Entry point, HTTP server | Flag parsing, signal handling, `/metrics`, `/healthz`, `/readyz`, `/sync` endpoints |
| `kubernetes.go` | Kube client setup | `setupKubernetes()` - config resolution, informer factory, cache sync with progress logging |
| `collector.go` | Prometheus collector | `Collect()` - computes metrics, `calculatePodRequest()` - init container logic |
| `Dockerfile` | Container image | Multi-stage: `golang:1.25-alpine` → `distroless/static-debian12:nonroot` |
| `charts/` | Helm deployment | RBAC (get/list/watch nodes+pods), ServiceMonitor, configurable resync period |
| `.github/workflows/` | CI/CD | `ci.yaml` - build/vet/lint, `release.yaml` - GHCR push on tag |
| `test-connectivity.sh` | Diagnostics | Validates kubeconfig, API connectivity, node/pod access |

## Metrics Exported

All metrics computed at scrape time from informer cache:

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `binpacking_node_allocated` | Gauge | `node`, `resource` | Total resource requests on node |
| `binpacking_node_allocatable` | Gauge | `node`, `resource` | Node capacity |
| `binpacking_node_utilization_ratio` | Gauge | `node`, `resource` | allocated / allocatable (0.0-1.0+) |
| `binpacking_cluster_allocated` | Gauge | `resource` | Cluster-wide total requests |
| `binpacking_cluster_allocatable` | Gauge | `resource` | Cluster-wide capacity |
| `binpacking_cluster_utilization_ratio` | Gauge | `resource` | Cluster-wide ratio |
| `binpacking_cache_age_seconds` | Gauge | - | Time since last informer sync |

## HTTP Endpoints

| Endpoint | Purpose | Returns |
|----------|---------|---------|
| `/` | Homepage | HTML page with links to all endpoints and configuration |
| `/metrics` | Prometheus scrape target | Text exposition format |
| `/healthz` | Liveness probe | 200 if process alive |
| `/readyz` | Readiness probe | 200 if cache synced, 503 otherwise |
| `/sync` | Cache status | JSON with last sync time, age, resync period, sync state |

## Build & Verify

```bash
# Build
go build -o kube-cluster-binpacking-exporter .

# Verify
go vet ./...
helm lint charts/kube-cluster-binpacking-exporter

# Test connectivity (run this first!)
./test-connectivity.sh

# Run locally
go run . --kubeconfig ~/.kube/config
go run . --kubeconfig ~/.kube/config --debug           # verbose
go run . --resync-period=1m --debug                     # fast resync
```

## Key Design Decisions

### Logging
- **`slog` stdlib**: Structured JSON logging, no external deps
- **Debug flag**: `--debug` enables verbose logging (pod filtering, resource calculations, informer events)
- **Conditional event handlers**: Only registered when debug enabled via `logger.Enabled(ctx, slog.LevelDebug)` — zero overhead in production

### Kubernetes Client
- **Config resolution order**: explicit flag → `~/.kube/config` → in-cluster
- **API connectivity test**: Calls `ServerVersion()` before setting up informers to fail fast
- **Progress logging during sync**: Updates every 5 seconds showing which informers have synced
- **Sync timeout**: 2-minute timeout prevents hanging forever on connection issues

### Pod Accounting
- **Init container logic**: `calculatePodRequest()` uses `max(sum_of_regular_containers, max_init_container)` — matches Kubernetes scheduler
- **Pod filtering**: Excludes `NodeName == ""` (unscheduled) and `Phase == Succeeded|Failed` (terminated) from binpacking calculations
- **Debug visibility**: Logs when init containers dominate resource reservation

### Metrics Design
- **Scrape-time computation**: Metrics created fresh on each scrape using `MustNewConstMetric` — automatically handles node add/remove
- **Custom registry**: Uses `prometheus.NewRegistry()` instead of `prometheus.DefaultRegistry` to avoid Go runtime metrics
- **Resource-agnostic labels**: `resource` label instead of separate metric per resource type (cpu, memory, etc.)
- **Cache age metric**: `binpacking_cache_age_seconds` enables alerting on stale cache

### Health Checks
- **Liveness vs Readiness**: `/healthz` checks process health, `/readyz` checks cache sync state
- **Readiness function**: `setupKubernetes()` returns closure that checks `HasSynced()` on both informers
- **Probe timing**: Readiness uses shorter delay/period (5s/10s), liveness uses longer (10s/30s)

### Informer Configuration
- **Configurable resync**: `--resync-period` flag (default 5m) controls how often informers refresh from API server
- **SharedInformerFactory**: Single watch connection shared between node and pod informers
- **SyncInfo tracking**: Records last sync time, exposes via `/sync` endpoint and `cache_age_seconds` metric

## Conventions

### Code Style
- **Flat structure**: No `pkg/` or `internal/` until genuinely needed
- **Package main**: All code in main package — this is a simple binary, not a library
- **Helper functions at top**: `calculatePodRequest()` defined before `Collect()` that uses it

### Naming
- **Flags**: Use `--kebab-case` (Go `flag` package standard)
- **Helm templates**: Use `binpacking-exporter.*` helper prefix for consistency
- **Metrics**: All prefixed with `binpacking_` for namespacing
- **Port**: 9101 (avoids collision with node-exporter on 9100, Prometheus on 9090)

### Configuration
- **Defaults optimized for production**: 5m resync, info logging, port 9101
- **Debug mode changes behavior**: Adds event handlers, increases log verbosity
- **Helm values mirror flags**: `debug`, `resyncPeriod`, `resources` directly map to CLI flags

## Dependencies

Minimal dependency footprint — only official Kubernetes and Prometheus libs:

| Package | Purpose | Version Constraint |
|---------|---------|-------------------|
| `k8s.io/client-go` | Kubernetes API client, informers, listers | Latest stable |
| `k8s.io/api` | Kubernetes resource types (Pod, Node, etc.) | Match client-go |
| `k8s.io/apimachinery` | Kubernetes primitives (Quantity, etc.) | Match client-go |
| `github.com/prometheus/client_golang` | Prometheus collector interface, HTTP handler | Latest stable |

No controller-runtime, no operator SDK — just the essentials.

## Troubleshooting

### Exporter won't start
1. Run `./test-connectivity.sh` to check kubeconfig
2. Check logs with `--debug` for detailed error messages
3. Verify RBAC permissions (needs get/list/watch on nodes and pods)

### Cache sync hangs
- Check API server connectivity: `kubectl cluster-info`
- Look for "still waiting for cache sync" debug logs showing which informer is stuck
- Informer sync timeout is 2 minutes — check if API server is responding slowly

### Metrics show zero
- Check `/readyz` — if not ready, cache hasn't synced yet
- Verify pods have resource requests defined (we measure requests, not limits)
- Enable `--debug` to see pod filtering decisions

### Cache age keeps growing
- Check `/sync` endpoint for sync state
- Verify API server watch connections aren't dropping
- Consider shorter `--resync-period` if needed

## Future Enhancements

See `TODO.md` for planned features:
- Per-node-label binpacking calculations
- Human-readable log output option with colors
- Unit tests with coverage reporting
- Event-handler based pre-computation for O(nodes) scrapes
- Paginated initial list for large clusters
