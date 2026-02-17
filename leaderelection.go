package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// LeaderElectionConfig holds the configuration for leader election.
type LeaderElectionConfig struct {
	LeaseName      string
	LeaseNamespace string
	Identity       string
	LeaseDuration  time.Duration
	RenewDeadline  time.Duration
	RetryPeriod    time.Duration
}

// serviceAccountNamespaceFile is the standard path where Kubernetes mounts
// the pod's namespace via the service account volume.
const serviceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// detectNamespace returns the override value if non-empty, otherwise reads from
// the downward API service account file. Returns an error if neither is available.
func detectNamespace(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	data, err := os.ReadFile(serviceAccountNamespaceFile)
	if err != nil {
		return "", fmt.Errorf("cannot detect namespace (not running in-cluster?): set --leader-election-namespace explicitly: %w", err)
	}
	ns := string(data)
	if ns == "" {
		return "", fmt.Errorf("namespace file %s is empty: set --leader-election-namespace explicitly", serviceAccountNamespaceFile)
	}
	return ns, nil
}

// detectIdentity returns the override value if non-empty, otherwise falls back
// to os.Hostname() which in Kubernetes equals the pod name.
func detectIdentity(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("cannot detect identity from hostname: set --leader-election-id explicitly: %w", err)
	}
	return hostname, nil
}

// runLeaderElection starts the leader election loop. It blocks until the context
// is cancelled. On leadership loss it calls os.Exit(0) — the standard Kubernetes
// pattern where the kubelet restarts the pod to re-enter the election.
func runLeaderElection(ctx context.Context, clientset kubernetes.Interface, config LeaderElectionConfig, isLeader *atomic.Bool, logger *slog.Logger) {
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      config.LeaseName,
			Namespace: config.LeaseNamespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: config.Identity,
		},
	}

	logger.Info("starting leader election",
		"lease", config.LeaseNamespace+"/"+config.LeaseName,
		"identity", config.Identity,
		"lease_duration", config.LeaseDuration,
		"renew_deadline", config.RenewDeadline,
		"retry_period", config.RetryPeriod)

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   config.LeaseDuration,
		RenewDeadline:   config.RenewDeadline,
		RetryPeriod:     config.RetryPeriod,
		ReleaseOnCancel: true,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				isLeader.Store(true)
				logger.Info("acquired leadership — publishing binpacking metrics")
			},
			OnStoppedLeading: func() {
				isLeader.Store(false)
				logger.Info("lost leadership — exiting to re-enter election")
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == config.Identity {
					return
				}
				logger.Info("current leader", "identity", identity)
			},
		},
	})
}
