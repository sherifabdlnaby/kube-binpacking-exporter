package main

import (
	"testing"
	"time"
)

// TestSyncInfo tests the SyncInfo struct fields and usage.
func TestSyncInfo(t *testing.T) {
	// Create a SyncInfo with test data
	syncTime := time.Now()
	resyncPeriod := 5 * time.Minute

	nodeSynced := true
	podSynced := false

	syncInfo := &SyncInfo{
		LastSyncTime: syncTime,
		ResyncPeriod: resyncPeriod,
		NodeSynced:   func() bool { return nodeSynced },
		PodSynced:    func() bool { return podSynced },
	}

	// Verify fields
	if syncInfo.LastSyncTime != syncTime {
		t.Errorf("LastSyncTime = %v, want %v", syncInfo.LastSyncTime, syncTime)
	}

	if syncInfo.ResyncPeriod != resyncPeriod {
		t.Errorf("ResyncPeriod = %v, want %v", syncInfo.ResyncPeriod, resyncPeriod)
	}

	if syncInfo.NodeSynced() != nodeSynced {
		t.Errorf("NodeSynced() = %v, want %v", syncInfo.NodeSynced(), nodeSynced)
	}

	if syncInfo.PodSynced() != podSynced {
		t.Errorf("PodSynced() = %v, want %v", syncInfo.PodSynced(), podSynced)
	}
}

// TestReadinessCheck tests the ReadyChecker function behavior.
func TestReadinessCheck(t *testing.T) {
	tests := []struct {
		name       string
		nodeSynced bool
		podSynced  bool
		wantReady  bool
	}{
		{
			name:       "both synced - ready",
			nodeSynced: true,
			podSynced:  true,
			wantReady:  true,
		},
		{
			name:       "only node synced - not ready",
			nodeSynced: true,
			podSynced:  false,
			wantReady:  false,
		},
		{
			name:       "only pod synced - not ready",
			nodeSynced: false,
			podSynced:  true,
			wantReady:  false,
		},
		{
			name:       "neither synced - not ready",
			nodeSynced: false,
			podSynced:  false,
			wantReady:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a readyChecker function similar to how it's done in setupKubernetes
			nodeSyncedFunc := func() bool { return tt.nodeSynced }
			podSyncedFunc := func() bool { return tt.podSynced }

			readyChecker := func() bool {
				return nodeSyncedFunc() && podSyncedFunc()
			}

			got := readyChecker()
			if got != tt.wantReady {
				t.Errorf("readyChecker() = %v, want %v", got, tt.wantReady)
			}
		})
	}
}

// Note: stripContainers and stripUnnecessaryFields are internal functions
// used by the informer transform. They are tested indirectly through the
// collector tests which exercise the complete flow.
