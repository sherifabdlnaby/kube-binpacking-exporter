package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	corev1 "k8s.io/api/core/v1"
)

var version = "dev"

func main() {
	var (
		kubeconfig   string
		metricsAddr  string
		metricsPath  string
		resourceCSV  string
		debug        bool
		resyncPeriod string
	)

	flag.StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (uses in-cluster config if empty)")
	flag.StringVar(&metricsAddr, "metrics-addr", ":9101", "address to serve metrics on")
	flag.StringVar(&metricsPath, "metrics-path", "/metrics", "HTTP path for metrics endpoint")
	flag.StringVar(&resourceCSV, "resources", "cpu,memory", "comma-separated list of resources to track")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.StringVar(&resyncPeriod, "resync-period", "5m", "informer cache resync period (e.g., 1m, 30s, 1h30m)")
	flag.Parse()

	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	logger.Info("starting kube-cluster-binpacking-exporter", "version", version, "debug", debug)

	resources := parseResources(resourceCSV)
	logger.Info("tracking resources", "resources", resourceCSV)

	resync, err := time.ParseDuration(resyncPeriod)
	if err != nil {
		logger.Error("invalid resync period", "error", err, "value", resyncPeriod)
		os.Exit(1)
	}
	logger.Info("informer resync period", "duration", resync)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	nodeLister, podLister, readyChecker, syncInfo, err := setupKubernetes(ctx, logger, kubeconfig, resync)
	if err != nil {
		logger.Error("failed to setup kubernetes client", "error", err)
		os.Exit(1)
	}

	collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, syncInfo)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	mux := http.NewServeMux()

	// Homepage - links to all endpoints
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Kube Cluster Binpacking Exporter</title>
    <style>
        body { font-family: system-ui, -apple-system, sans-serif; max-width: 800px; margin: 40px auto; padding: 0 20px; line-height: 1.6; }
        h1 { color: #333; border-bottom: 2px solid #007bff; padding-bottom: 10px; }
        h2 { color: #555; margin-top: 30px; }
        .endpoint { background: #f8f9fa; padding: 15px; margin: 10px 0; border-radius: 5px; border-left: 4px solid #007bff; }
        .endpoint-title { font-weight: bold; color: #007bff; font-size: 1.1em; }
        .endpoint-desc { margin-top: 5px; color: #666; }
        a { color: #007bff; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .version { color: #999; font-size: 0.9em; }
    </style>
</head>
<body>
    <h1>Kube Cluster Binpacking Exporter</h1>
    <p class="version">Version: %s</p>
    <p>Prometheus exporter for Kubernetes cluster binpacking efficiency metrics. Tracks resource allocation by comparing pod requests against node allocatable capacity.</p>

    <h2>Endpoints</h2>

    <div class="endpoint">
        <div class="endpoint-title"><a href="%s">%s</a></div>
        <div class="endpoint-desc">Prometheus metrics endpoint. Exports binpacking efficiency metrics for CPU, memory, and other resources.</div>
    </div>

    <div class="endpoint">
        <div class="endpoint-title"><a href="/sync">/sync</a></div>
        <div class="endpoint-desc">Cache synchronization status. Shows last sync time, cache age, resync period, and per-informer sync state (JSON).</div>
    </div>

    <div class="endpoint">
        <div class="endpoint-title"><a href="/healthz">/healthz</a></div>
        <div class="endpoint-desc">Liveness probe. Returns 200 if the process is alive.</div>
    </div>

    <div class="endpoint">
        <div class="endpoint-title"><a href="/readyz">/readyz</a></div>
        <div class="endpoint-desc">Readiness probe. Returns 200 if informer cache is synced and ready to serve metrics, 503 otherwise.</div>
    </div>

    <h2>Configuration</h2>
    <ul>
        <li><strong>Resources tracked:</strong> %s</li>
        <li><strong>Resync period:</strong> %s</li>
        <li><strong>Debug mode:</strong> %t</li>
    </ul>

    <h2>Links</h2>
    <ul>
        <li><a href="https://github.com/sherifabdlnaby/kube-cluster-binpacking-exporter" target="_blank">GitHub Repository</a></li>
        <li><a href="https://prometheus.io/docs/instrumenting/exporters/" target="_blank">Prometheus Exporters Documentation</a></li>
    </ul>
</body>
</html>`, version, metricsPath, metricsPath, resourceCSV, resyncPeriod, debug)
	})

	mux.Handle(metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	// Liveness probe - checks if process is alive
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Readiness probe - checks if informer cache is synced
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if readyChecker() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ready")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintln(w, "not ready: informer cache not synced")
		}
	})

	// Sync status endpoint - shows cache sync information
	mux.HandleFunc("/sync", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
  "last_sync": "%s",
  "sync_age_seconds": %.0f,
  "resync_period": "%s",
  "node_synced": %t,
  "pod_synced": %t
}`,
			syncInfo.LastSyncTime.Format(time.RFC3339),
			time.Since(syncInfo.LastSyncTime).Seconds(),
			syncInfo.ResyncPeriod,
			syncInfo.NodeSynced(),
			syncInfo.PodSynced())
	})

	srv := &http.Server{
		Addr:              metricsAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("serving metrics", "addr", metricsAddr, "path", metricsPath)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown error", "error", err)
	}
}

func parseResources(csv string) []corev1.ResourceName {
	parts := strings.Split(csv, ",")
	resources := make([]corev1.ResourceName, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			resources = append(resources, corev1.ResourceName(p))
		}
	}
	return resources
}
