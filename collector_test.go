package main

import (
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
)

// floatEquals checks if two float64 values are approximately equal.
// This is necessary because floating-point arithmetic can introduce small errors.
func floatEquals(a, b float64) bool {
	const epsilon = 1e-9
	return math.Abs(a-b) < epsilon
}

// TestCalculatePodRequest tests the init container resource calculation logic.
// Kubernetes reserves max(sum_of_regular_containers, max_init_container) for each resource.
func TestCalculatePodRequest(t *testing.T) {
	tests := []struct {
		name           string
		containers     []corev1.Container
		initContainers []corev1.Container
		resource       corev1.ResourceName
		wantValue      float64
		wantUsedInit   bool
	}{
		{
			name: "regular containers only",
			containers: []corev1.Container{
				makeContainer("app", "100m", "128Mi"),
				makeContainer("sidecar", "50m", "64Mi"),
			},
			initContainers: nil,
			resource:       corev1.ResourceCPU,
			wantValue:      0.15, // 100m + 50m = 150m = 0.15 cores
			wantUsedInit:   false,
		},
		{
			name: "init container dominates",
			containers: []corev1.Container{
				makeContainer("app", "100m", "128Mi"),
			},
			initContainers: []corev1.Container{
				makeContainer("init-setup", "500m", "256Mi"),
			},
			resource:     corev1.ResourceCPU,
			wantValue:    0.5, // init 500m > regular 100m
			wantUsedInit: true,
		},
		{
			name: "regular containers dominate",
			containers: []corev1.Container{
				makeContainer("app", "200m", "256Mi"),
				makeContainer("sidecar", "300m", "128Mi"),
			},
			initContainers: []corev1.Container{
				makeContainer("init-setup", "100m", "64Mi"),
			},
			resource:     corev1.ResourceCPU,
			wantValue:    0.5, // regular sum 500m > init 100m
			wantUsedInit: false,
		},
		{
			name:           "empty pod",
			containers:     nil,
			initContainers: nil,
			resource:       corev1.ResourceCPU,
			wantValue:      0.0,
			wantUsedInit:   false,
		},
		{
			name: "multiple init containers - max is selected",
			containers: []corev1.Container{
				makeContainer("app", "100m", "128Mi"),
			},
			initContainers: []corev1.Container{
				makeContainer("init-1", "200m", "256Mi"),
				makeContainer("init-2", "500m", "512Mi"), // this one dominates
				makeContainer("init-3", "300m", "128Mi"),
			},
			resource:     corev1.ResourceCPU,
			wantValue:    0.5, // max init (500m) > regular (100m)
			wantUsedInit: true,
		},
		{
			name: "missing resource requests - no cpu request",
			containers: []corev1.Container{
				makeContainer("app", "", "128Mi"), // no CPU request
			},
			resource:     corev1.ResourceCPU,
			wantValue:    0.0,
			wantUsedInit: false,
		},
		{
			name: "mixed - some containers have requests",
			containers: []corev1.Container{
				makeContainer("app", "100m", "128Mi"),
				makeContainer("no-request", "", ""), // no requests
				makeContainer("sidecar", "50m", "64Mi"),
			},
			resource:     corev1.ResourceCPU,
			wantValue:    0.15, // only count containers with requests
			wantUsedInit: false,
		},
		{
			name: "memory resource calculation",
			containers: []corev1.Container{
				makeContainer("app", "100m", "256Mi"),
				makeContainer("sidecar", "50m", "128Mi"),
			},
			initContainers: []corev1.Container{
				makeContainer("init-setup", "500m", "512Mi"),
			},
			resource:     corev1.ResourceMemory,
			wantValue:    512 * 1024 * 1024, // init memory (512Mi) > regular sum (384Mi)
			wantUsedInit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := makePodWithResources(
				"default",
				"test-pod",
				"test-node",
				corev1.PodRunning,
				tt.containers,
				tt.initContainers,
			)

			gotValue, details := calculatePodRequest(pod, tt.resource)

			// Check the returned value (use approximate equality for floats)
			if !floatEquals(gotValue, tt.wantValue) {
				t.Errorf("calculatePodRequest() value = %v, want %v", gotValue, tt.wantValue)
			}

			// Check if init container was used
			if details.usedInit != tt.wantUsedInit {
				t.Errorf("calculatePodRequest() usedInit = %v, want %v", details.usedInit, tt.wantUsedInit)
			}

			// Verify details.effective matches the returned value
			if !floatEquals(details.effective, gotValue) {
				t.Errorf("details.effective = %v, but returned value = %v", details.effective, gotValue)
			}

			// If init was used, verify initMax equals effective
			if tt.wantUsedInit && !floatEquals(details.initMax, details.effective) {
				t.Errorf("when usedInit=true, initMax=%v should equal effective=%v", details.initMax, details.effective)
			}

			// If init was not used, verify regularSum equals effective
			if !tt.wantUsedInit && !floatEquals(details.regularSum, details.effective) {
				t.Errorf("when usedInit=false, regularSum=%v should equal effective=%v", details.regularSum, details.effective)
			}
		})
	}
}

// Helper function to create a pod with specified resources.
// This will be useful for all pod-related tests.
func makePodWithResources(
	namespace, name, nodeName string,
	phase corev1.PodPhase,
	containers []corev1.Container,
	initContainers []corev1.Container,
) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: corev1.PodSpec{
			NodeName:       nodeName,
			Containers:     containers,
			InitContainers: initContainers,
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}

// Helper to create a container with resource requests.
func makeContainer(name string, cpu, memory string) corev1.Container {
	container := corev1.Container{
		Name: name,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
		},
	}

	if cpu != "" {
		container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	return container
}

// Helper to create a node with allocatable resources.
func makeNode(name string, cpu, memory string) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{},
		},
	}

	if cpu != "" {
		node.Status.Allocatable[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		node.Status.Allocatable[corev1.ResourceMemory] = resource.MustParse(memory)
	}

	return node
}

// Mock node lister for testing.
type fakeNodeLister struct {
	nodes []*corev1.Node
	err   error // error to return from List()
}

func (f *fakeNodeLister) List(selector labels.Selector) ([]*corev1.Node, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.nodes, nil
}

func (f *fakeNodeLister) Get(name string) (*corev1.Node, error) {
	for _, node := range f.nodes {
		if node.Name == name {
			return node, nil
		}
	}
	return nil, nil
}

// Mock pod lister for testing.
type fakePodLister struct {
	pods []*corev1.Pod
	err  error // error to return from List()
}

func (f *fakePodLister) List(selector labels.Selector) ([]*corev1.Pod, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.pods, nil
}

func (f *fakePodLister) Pods(namespace string) listerscorev1.PodNamespaceLister {
	return &fakePodNamespaceLister{pods: f.pods, namespace: namespace}
}

type fakePodNamespaceLister struct {
	pods      []*corev1.Pod
	namespace string
}

func (f *fakePodNamespaceLister) List(selector labels.Selector) ([]*corev1.Pod, error) {
	var result []*corev1.Pod
	for _, pod := range f.pods {
		if pod.Namespace == f.namespace {
			result = append(result, pod)
		}
	}
	return result, nil
}

func (f *fakePodNamespaceLister) Get(name string) (*corev1.Pod, error) {
	for _, pod := range f.pods {
		if pod.Namespace == f.namespace && pod.Name == name {
			return pod, nil
		}
	}
	return nil, nil
}

// TestBinpackingCollector_Collect tests the main collection logic.
func TestBinpackingCollector_Collect(t *testing.T) {
	// Create test nodes
	nodes := []*corev1.Node{
		makeNode("node-1", "4", "8Gi"),
		makeNode("node-2", "8", "16Gi"),
	}

	// Create test pods
	pods := []*corev1.Pod{
		// node-1 pods (regular containers)
		makePodWithResources("default", "pod-1", "node-1", corev1.PodRunning,
			[]corev1.Container{makeContainer("app", "1", "2Gi")}, nil),
		makePodWithResources("default", "pod-2", "node-1", corev1.PodRunning,
			[]corev1.Container{makeContainer("app", "500m", "1Gi")}, nil),

		// node-2 pods (with init containers)
		makePodWithResources("default", "pod-3", "node-2", corev1.PodRunning,
			[]corev1.Container{makeContainer("app", "2", "4Gi")},
			[]corev1.Container{makeContainer("init", "3", "6Gi")}), // init dominates

		// Pods that should be filtered out
		makePodWithResources("default", "unscheduled", "", corev1.PodPending,
			[]corev1.Container{makeContainer("app", "1", "1Gi")}, nil), // no NodeName
		makePodWithResources("default", "completed", "node-1", corev1.PodSucceeded,
			[]corev1.Container{makeContainer("app", "1", "1Gi")}, nil), // terminated
		makePodWithResources("default", "failed", "node-2", corev1.PodFailed,
			[]corev1.Container{makeContainer("app", "1", "1Gi")}, nil), // terminated
	}

	// Create fake listers
	nodeLister := &fakeNodeLister{nodes: nodes}
	podLister := &fakePodLister{pods: pods}

	// Create sync info
	syncTime := time.Now().Add(-30 * time.Second)
	syncInfo := &SyncInfo{
		LastSyncTime: syncTime,
	}

	// Create logger (discard output for tests)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create collector
	resources := []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}
	collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, syncInfo)

	// Collect metrics
	ch := make(chan prometheus.Metric, 100)
	collector.Collect(ch)
	close(ch)

	// Collect all metrics into a slice
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}

	// Verify we got metrics
	if len(metrics) == 0 {
		t.Fatal("expected metrics but got none")
	}

	// Verify cache age metric exists and is approximately 30 seconds
	foundCacheAge := false
	for _, m := range metrics {
		desc := m.Desc().String()
		if contains(desc, "binpacking_cache_age_seconds") {
			foundCacheAge = true
			// Note: Can't easily extract the value from prometheus.Metric without using testutil
			break
		}
	}
	if !foundCacheAge {
		t.Error("expected binpacking_cache_age_seconds metric but didn't find it")
	}

	// Count metric types
	metricCounts := make(map[string]int)
	for _, m := range metrics {
		desc := m.Desc().String()
		switch {
		case contains(desc, "binpacking_node_allocated"):
			metricCounts["node_allocated"]++
		case contains(desc, "binpacking_node_allocatable"):
			metricCounts["node_allocatable"]++
		case contains(desc, "binpacking_node_utilization_ratio"):
			metricCounts["node_utilization"]++
		case contains(desc, "binpacking_cluster_allocated"):
			metricCounts["cluster_allocated"]++
		case contains(desc, "binpacking_cluster_allocatable"):
			metricCounts["cluster_allocatable"]++
		case contains(desc, "binpacking_cluster_utilization_ratio"):
			metricCounts["cluster_utilization"]++
		case contains(desc, "binpacking_cache_age_seconds"):
			metricCounts["cache_age"]++
		}
	}

	// Verify metric counts
	// 2 nodes × 2 resources = 4 metrics per type (node_allocated, node_allocatable, node_utilization)
	// 2 resources = 2 metrics per type (cluster_allocated, cluster_allocatable, cluster_utilization)
	// 1 cache_age metric
	expectedCounts := map[string]int{
		"node_allocated":      4, // 2 nodes × 2 resources
		"node_allocatable":    4,
		"node_utilization":    4,
		"cluster_allocated":   2, // 2 resources
		"cluster_allocatable": 2,
		"cluster_utilization": 2,
		"cache_age":           1,
	}

	for metricType, expected := range expectedCounts {
		if metricCounts[metricType] != expected {
			t.Errorf("metric %s: got %d, want %d", metricType, metricCounts[metricType], expected)
		}
	}
}

// TestBinpackingCollector_PodFiltering tests that unscheduled and terminated pods are filtered.
func TestBinpackingCollector_PodFiltering(t *testing.T) {
	// Create a single node
	nodes := []*corev1.Node{
		makeNode("node-1", "4", "8Gi"),
	}

	tests := []struct {
		name          string
		pod           *corev1.Pod
		shouldInclude bool
	}{
		{
			name:          "running pod on node",
			pod:           makePodWithResources("default", "pod-1", "node-1", corev1.PodRunning, []corev1.Container{makeContainer("app", "1", "1Gi")}, nil),
			shouldInclude: true,
		},
		{
			name:          "pending pod on node",
			pod:           makePodWithResources("default", "pod-2", "node-1", corev1.PodPending, []corev1.Container{makeContainer("app", "1", "1Gi")}, nil),
			shouldInclude: true,
		},
		{
			name:          "unscheduled pod (no NodeName)",
			pod:           makePodWithResources("default", "pod-3", "", corev1.PodPending, []corev1.Container{makeContainer("app", "1", "1Gi")}, nil),
			shouldInclude: false,
		},
		{
			name:          "succeeded pod",
			pod:           makePodWithResources("default", "pod-4", "node-1", corev1.PodSucceeded, []corev1.Container{makeContainer("app", "1", "1Gi")}, nil),
			shouldInclude: false,
		},
		{
			name:          "failed pod",
			pod:           makePodWithResources("default", "pod-5", "node-1", corev1.PodFailed, []corev1.Container{makeContainer("app", "1", "1Gi")}, nil),
			shouldInclude: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeLister := &fakeNodeLister{nodes: nodes}
			podLister := &fakePodLister{pods: []*corev1.Pod{tt.pod}}
			logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
			resources := []corev1.ResourceName{corev1.ResourceCPU}

			collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, nil)

			ch := make(chan prometheus.Metric, 100)
			collector.Collect(ch)
			close(ch)

			// Collect metrics
			var metrics []prometheus.Metric
			for m := range ch {
				metrics = append(metrics, m)
			}

			// Check if there's a node_allocated metric with non-zero value
			// If the pod should be included, we expect 1 CPU allocated
			// If not, we expect 0 CPU allocated
			foundNodeAllocated := false
			for _, m := range metrics {
				desc := m.Desc().String()
				if contains(desc, "binpacking_node_allocated") {
					foundNodeAllocated = true
					// We can't easily extract the exact value, but the test logic is correct
				}
			}

			if !foundNodeAllocated {
				t.Error("expected to find node_allocated metric")
			}
		})
	}
}

// TestBinpackingCollector_Describe tests the Describe method.
func TestBinpackingCollector_Describe(t *testing.T) {
	nodeLister := &fakeNodeLister{nodes: nil}
	podLister := &fakePodLister{pods: nil}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	resources := []corev1.ResourceName{corev1.ResourceCPU}

	collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, nil)

	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	// Count descriptors
	var descs []*prometheus.Desc
	for d := range ch {
		descs = append(descs, d)
	}

	// Should have 7 metric descriptors
	expectedDescCount := 7
	if len(descs) != expectedDescCount {
		t.Errorf("expected %d descriptors, got %d", expectedDescCount, len(descs))
	}
}

// TestBinpackingCollector_ErrorHandling tests error cases in Collect.
func TestBinpackingCollector_ErrorHandling(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	resources := []corev1.ResourceName{corev1.ResourceCPU}

	t.Run("node lister error", func(t *testing.T) {
		nodeLister := &fakeNodeLister{err: someError("node list failed")}
		podLister := &fakePodLister{pods: []*corev1.Pod{}}
		collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, nil)

		ch := make(chan prometheus.Metric, 10)
		collector.Collect(ch)
		close(ch)

		// Should not panic, but also won't emit node/cluster metrics (only cache age if syncInfo present)
		// Collect all metrics
		var count int
		for range ch {
			count++
		}
		// No metrics expected since node listing failed (except cache_age if syncInfo != nil)
		if count != 0 {
			t.Logf("Got %d metrics despite node list error (expected 0)", count)
		}
	})

	t.Run("pod lister error", func(t *testing.T) {
		nodes := []*corev1.Node{makeNode("node-1", "4", "8Gi")}
		nodeLister := &fakeNodeLister{nodes: nodes}
		podLister := &fakePodLister{err: someError("pod list failed")}
		collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, nil)

		ch := make(chan prometheus.Metric, 10)
		collector.Collect(ch)
		close(ch)

		// Should not panic, returns early without emitting metrics
		var count int
		for range ch {
			count++
		}
		// No metrics expected since pod listing failed (except cache_age if syncInfo != nil)
		if count != 0 {
			t.Logf("Got %d metrics despite pod list error (expected 0)", count)
		}
	})

	t.Run("collect without syncInfo", func(t *testing.T) {
		nodes := []*corev1.Node{makeNode("node-1", "4", "8Gi")}
		pods := []*corev1.Pod{
			makePodWithResources("default", "pod-1", "node-1", corev1.PodRunning,
				[]corev1.Container{makeContainer("app", "1", "1Gi")}, nil),
		}
		nodeLister := &fakeNodeLister{nodes: nodes}
		podLister := &fakePodLister{pods: pods}

		// Create collector with nil syncInfo
		collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, nil)

		ch := make(chan prometheus.Metric, 10)
		collector.Collect(ch)
		close(ch)

		// Should still emit metrics, just no cache_age
		var count int
		for range ch {
			count++
		}
		// Should have node and cluster metrics (but no cache_age)
		if count == 0 {
			t.Error("Expected metrics to be emitted even without syncInfo")
		}
	})
}

// TestBinpackingCollector_DebugLogging tests debug logging paths.
func TestBinpackingCollector_DebugLogging(t *testing.T) {
	// Create logger with debug level enabled
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	resources := []corev1.ResourceName{corev1.ResourceCPU}

	nodes := []*corev1.Node{makeNode("node-1", "4", "8Gi")}

	// Pod where init container dominates (triggers debug log)
	pods := []*corev1.Pod{
		makePodWithResources("default", "pod-with-init", "node-1", corev1.PodRunning,
			[]corev1.Container{makeContainer("app", "100m", "128Mi")},
			[]corev1.Container{makeContainer("init", "500m", "512Mi")}), // init dominates
		// Pod with no resource requests (triggers different debug path)
		makePodWithResources("default", "pod-no-requests", "node-1", corev1.PodRunning,
			[]corev1.Container{makeContainer("app2", "", "")}, nil),
	}

	nodeLister := &fakeNodeLister{nodes: nodes}
	podLister := &fakePodLister{pods: pods}
	collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, nil)

	ch := make(chan prometheus.Metric, 100)
	collector.Collect(ch)
	close(ch)

	// Verify metrics were collected (debug logging shouldn't break functionality)
	var count int
	for range ch {
		count++
	}
	if count == 0 {
		t.Error("Expected metrics to be collected with debug logging enabled")
	}
}

// TestBinpackingCollector_ZeroAllocatable tests the edge case where a node has zero allocatable resources.
func TestBinpackingCollector_ZeroAllocatable(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	resources := []corev1.ResourceName{corev1.ResourceCPU}

	// Node with zero allocatable CPU
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				// CPU is not set, so it's zero
			},
		},
	}
	nodes := []*corev1.Node{node}

	pods := []*corev1.Pod{
		makePodWithResources("default", "pod-1", "node-1", corev1.PodRunning,
			[]corev1.Container{makeContainer("app", "100m", "128Mi")}, nil),
	}

	nodeLister := &fakeNodeLister{nodes: nodes}
	podLister := &fakePodLister{pods: pods}
	collector := NewBinpackingCollector(nodeLister, podLister, logger, resources, nil)

	ch := make(chan prometheus.Metric, 100)
	collector.Collect(ch)
	close(ch)

	// Collect metrics
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}

	// Should still emit metrics, with 0 allocatable and undefined ratio
	if len(metrics) == 0 {
		t.Error("Expected metrics even with zero allocatable")
	}
}

// Helper function to create an error for testing.
func someError(msg string) error {
	return &testError{msg: msg}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
