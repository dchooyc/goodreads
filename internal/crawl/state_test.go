package crawl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-state.json")

	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("load fresh state: %v", err)
	}
	if s.IsDone("1") {
		t.Fatal("fresh state should have no completed targets")
	}

	if err := s.Update("1", StatusRecovered, []string{"https://example.com/a"}, ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := s.Update("2", StatusHTTPFailed, []string{"https://example.com/b"}, "boom"); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Simulate a killed and restarted run: reload from disk.
	resumed, err := LoadState(path)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !resumed.IsDone("1") {
		t.Error("recovered target must be skipped on resume")
	}
	if resumed.IsDone("2") {
		t.Error("http_failed target must be retried on resume")
	}

	ts, ok := resumed.Get("2")
	if !ok || ts.LastError != "boom" || ts.UpdatedAt == "" {
		t.Errorf("state record incomplete: %+v", ts)
	}
}

func TestStateWritesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recovery-state.json")

	s, _ := LoadState(path)
	if err := s.Update("1", StatusRecovered, nil, ""); err != nil {
		t.Fatalf("update: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
