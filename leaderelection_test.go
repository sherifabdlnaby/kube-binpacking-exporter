package main

import (
	"os"
	"testing"
)

func TestDetectNamespace(t *testing.T) {
	t.Run("override provided", func(t *testing.T) {
		ns, err := detectNamespace("my-namespace")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ns != "my-namespace" {
			t.Errorf("expected %q, got %q", "my-namespace", ns)
		}
	})

	t.Run("empty override and no service account file", func(t *testing.T) {
		// This test assumes the CI/local environment doesn't have the
		// in-cluster service account namespace file.
		_, err := detectNamespace("")
		if err == nil {
			// If we're running in-cluster this would succeed; skip in that case
			if _, statErr := os.Stat(serviceAccountNamespaceFile); statErr == nil {
				t.Skip("running in-cluster, skipping namespace detection failure test")
			}
			t.Error("expected error when no override and no SA file, got nil")
		}
	})
}

func TestDetectIdentity(t *testing.T) {
	t.Run("override provided", func(t *testing.T) {
		id, err := detectIdentity("my-pod-name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != "my-pod-name" {
			t.Errorf("expected %q, got %q", "my-pod-name", id)
		}
	})

	t.Run("empty override falls back to hostname", func(t *testing.T) {
		id, err := detectIdentity("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		hostname, _ := os.Hostname()
		if id != hostname {
			t.Errorf("expected hostname %q, got %q", hostname, id)
		}
	})
}
