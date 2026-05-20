package witness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestRecordBeadRespawn_Increments(t *testing.T) {
	tmpDir := t.TempDir()
	// Create the witness subdirectory so the state file path is valid.
	if err := os.MkdirAll(filepath.Join(tmpDir, "witness"), 0755); err != nil {
		t.Fatal(err)
	}

	count := RecordBeadRespawn(tmpDir, "bead-1")
	if count != 1 {
		t.Errorf("first RecordBeadRespawn = %d, want 1", count)
	}

	count = RecordBeadRespawn(tmpDir, "bead-1")
	if count != 2 {
		t.Errorf("second RecordBeadRespawn = %d, want 2", count)
	}
}

func TestRecordBeadRespawnAttempt_RecordsBoundedRecentContext(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "witness"), 0755); err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= maxRecentRespawnAttempts+1; i++ {
		count := RecordBeadRespawnAttempt(tmpDir, "bead-history", fmt.Sprintf("attempt-%d", i))
		if count != i {
			t.Fatalf("RecordBeadRespawnAttempt count = %d, want %d", count, i)
		}
	}

	state := loadBeadRespawnState(tmpDir)
	rec := state.Beads["bead-history"]
	if rec == nil {
		t.Fatal("bead-history record not found")
	}
	if len(rec.RecentAttempts) != maxRecentRespawnAttempts {
		t.Fatalf("recent attempts length = %d, want %d", len(rec.RecentAttempts), maxRecentRespawnAttempts)
	}
	if rec.RecentAttempts[0].Reason != "attempt-2" {
		t.Errorf("oldest retained reason = %q, want attempt-2", rec.RecentAttempts[0].Reason)
	}
	if rec.RecentAttempts[len(rec.RecentAttempts)-1].Timestamp.IsZero() {
		t.Error("recent attempt timestamp was not recorded")
	}

	summary := RecentBeadRespawnSummary(tmpDir, "bead-history")
	if strings.Contains(summary, "attempt-1") {
		t.Errorf("summary contains trimmed attempt: %q", summary)
	}
	if !strings.Contains(summary, "attempt-6") {
		t.Errorf("summary missing newest attempt: %q", summary)
	}
}

func TestRecentBeadRespawnSummary_UnknownBead(t *testing.T) {
	tmpDir := t.TempDir()
	if got := RecentBeadRespawnSummary(tmpDir, "missing"); got != "none recorded" {
		t.Errorf("RecentBeadRespawnSummary = %q, want none recorded", got)
	}
}

func TestShouldBlockRespawn_Threshold(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "witness"), 0755); err != nil {
		t.Fatal(err)
	}

	// Below threshold.
	for i := 0; i < config.DefaultWitnessMaxBeadRespawns-1; i++ {
		RecordBeadRespawn(tmpDir, "bead-2")
	}
	if ShouldBlockRespawn(tmpDir, "bead-2") {
		t.Error("ShouldBlockRespawn = true before reaching threshold")
	}

	// At threshold.
	RecordBeadRespawn(tmpDir, "bead-2")
	if !ShouldBlockRespawn(tmpDir, "bead-2") {
		t.Error("ShouldBlockRespawn = false at threshold")
	}
}

func TestResetBeadRespawnCount(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "witness"), 0755); err != nil {
		t.Fatal(err)
	}

	RecordBeadRespawn(tmpDir, "bead-3")
	RecordBeadRespawn(tmpDir, "bead-3")

	if err := ResetBeadRespawnCount(tmpDir, "bead-3"); err != nil {
		t.Fatalf("ResetBeadRespawnCount error: %v", err)
	}

	if ShouldBlockRespawn(tmpDir, "bead-3") {
		t.Error("ShouldBlockRespawn = true after reset")
	}

	// Re-increment should start from 1.
	count := RecordBeadRespawn(tmpDir, "bead-3")
	if count != 1 {
		t.Errorf("RecordBeadRespawn after reset = %d, want 1", count)
	}
}

func TestRecordBeadRespawn_ConcurrentSafe(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "witness"), 0755); err != nil {
		t.Fatal(err)
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			RecordBeadRespawn(tmpDir, "bead-race")
		}()
	}
	wg.Wait()

	// After all goroutines, the count must equal the number of increments.
	state := loadBeadRespawnState(tmpDir)
	rec, ok := state.Beads["bead-race"]
	if !ok {
		t.Fatal("bead-race record not found")
	}
	if rec.Count != goroutines {
		t.Errorf("concurrent count = %d, want %d", rec.Count, goroutines)
	}
}

func TestShouldBlockRespawn_UnknownBead(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "witness"), 0755); err != nil {
		t.Fatal(err)
	}

	if ShouldBlockRespawn(tmpDir, "nonexistent") {
		t.Error("ShouldBlockRespawn = true for unknown bead")
	}
}
