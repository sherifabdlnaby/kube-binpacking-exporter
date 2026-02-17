package main

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestCalculatePodRequest tests the init container resource calculation logic.
// This will be implemented in Task #2.
func TestCalculatePodRequest(t *testing.T) {
	t.Run("regular containers only", func(t *testing.T) {
		t.Skip("TODO: implement in Task #2")
	})

	t.Run("init container dominates", func(t *testing.T) {
		t.Skip("TODO: implement in Task #2")
	})

	t.Run("regular containers dominate", func(t *testing.T) {
		t.Skip("TODO: implement in Task #2")
	})

	t.Run("empty pod", func(t *testing.T) {
		t.Skip("TODO: implement in Task #2")
	})

	t.Run("multiple init containers", func(t *testing.T) {
		t.Skip("TODO: implement in Task #2")
	})

	t.Run("missing resource requests", func(t *testing.T) {
		t.Skip("TODO: implement in Task #2")
	})
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
