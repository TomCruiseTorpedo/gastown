package cmd

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/crew"
)

func TestParseCrewTarget(t *testing.T) {
	tests := []struct {
		target   string
		wantRig  string
		wantName string
		wantOk   bool
	}{
		{"gastown/crew/mel", "gastown", "mel", true},
		{"beads/crew/dave", "beads", "dave", true},
		{"gastown/polecats/foo", "", "", false},
		{"gastown/witness", "", "", false},
		{"gastown", "", "", false},
		{"gastown/crew/", "", "", false},
		{"/crew/mel", "", "", false},
		{"gastown/crew/mel/extra", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rig, name, ok := parseCrewTarget(tt.target)
			if rig != tt.wantRig || name != tt.wantName || ok != tt.wantOk {
				t.Errorf("parseCrewTarget(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.target, rig, name, ok, tt.wantRig, tt.wantName, tt.wantOk)
			}
		})
	}
}

// TestResolveTarget_StoppedCrewAutoStart verifies that when the target is a
// stopped crew member, resolveTarget auto-starts them and retries instead of
// surfacing the opaque "getting pane for gt-crew-X" tmux error (gt-028y).
func TestResolveTarget_StoppedCrewAutoStart(t *testing.T) {
	prevResolve := resolveTargetAgentFn
	prevStart := startStoppedCrewFn
	t.Cleanup(func() {
		resolveTargetAgentFn = prevResolve
		startStoppedCrewFn = prevStart
	})

	startCalls := 0
	resolveCalls := 0
	resolveTargetAgentFn = func(target string) (string, string, string, error) {
		resolveCalls++
		if resolveCalls == 1 {
			return "", "", "", fmt.Errorf("getting pane for gt-crew-mel: exit status 1")
		}
		return "gastown/crew/mel", "%42", "/work/dir", nil
	}
	startStoppedCrewFn = func(rigName, crewName, townRoot string) error {
		startCalls++
		if rigName != "gastown" || crewName != "mel" {
			t.Errorf("start called with (%q, %q), want (gastown, mel)", rigName, crewName)
		}
		return nil
	}

	got, err := resolveTarget("gastown/crew/mel", ResolveTargetOptions{TownRoot: "/town"})
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if startCalls != 1 {
		t.Errorf("startStoppedCrewFn called %d times, want 1", startCalls)
	}
	if resolveCalls != 2 {
		t.Errorf("resolveTargetAgentFn called %d times, want 2", resolveCalls)
	}
	if got.Agent != "gastown/crew/mel" || got.Pane != "%42" || got.WorkDir != "/work/dir" {
		t.Errorf("unexpected resolved target: %+v", got)
	}
}

// TestResolveTarget_StoppedCrewMissing verifies the clear error when the
// target crew member doesn't exist on disk.
func TestResolveTarget_StoppedCrewMissing(t *testing.T) {
	prevResolve := resolveTargetAgentFn
	prevStart := startStoppedCrewFn
	t.Cleanup(func() {
		resolveTargetAgentFn = prevResolve
		startStoppedCrewFn = prevStart
	})

	resolveTargetAgentFn = func(target string) (string, string, string, error) {
		return "", "", "", fmt.Errorf("getting pane for gt-crew-ghost: exit status 1")
	}
	startStoppedCrewFn = func(rigName, crewName, townRoot string) error {
		return crew.ErrCrewNotFound
	}

	_, err := resolveTarget("gastown/crew/ghost", ResolveTargetOptions{TownRoot: "/town"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gt crew add ghost") {
		t.Errorf("expected actionable 'gt crew add' guidance, got: %v", err)
	}
}

// TestResolveTarget_StoppedCrewStartFails verifies that auto-start failures
// surface a clear error with manual-start guidance.
func TestResolveTarget_StoppedCrewStartFails(t *testing.T) {
	prevResolve := resolveTargetAgentFn
	prevStart := startStoppedCrewFn
	t.Cleanup(func() {
		resolveTargetAgentFn = prevResolve
		startStoppedCrewFn = prevStart
	})

	resolveTargetAgentFn = func(target string) (string, string, string, error) {
		return "", "", "", fmt.Errorf("getting pane for gt-crew-mel: exit status 1")
	}
	startStoppedCrewFn = func(rigName, crewName, townRoot string) error {
		return errors.New("tmux refused: terminal too small")
	}

	_, err := resolveTarget("gastown/crew/mel", ResolveTargetOptions{TownRoot: "/town"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "auto-start failed") {
		t.Errorf("expected 'auto-start failed' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gt crew start mel") {
		t.Errorf("expected manual-start guidance, got: %v", err)
	}
}

// TestResolveTarget_StoppedCrewDryRun verifies that DryRun skips auto-start
// (it must not have side effects).
func TestResolveTarget_StoppedCrewDryRun(t *testing.T) {
	prevResolve := resolveTargetAgentFn
	prevStart := startStoppedCrewFn
	t.Cleanup(func() {
		resolveTargetAgentFn = prevResolve
		startStoppedCrewFn = prevStart
	})

	resolveTargetAgentFn = func(target string) (string, string, string, error) {
		return "", "", "", fmt.Errorf("getting pane for gt-crew-mel: exit status 1")
	}
	startStoppedCrewFn = func(rigName, crewName, townRoot string) error {
		t.Fatal("startStoppedCrewFn should not be called in DryRun mode")
		return nil
	}

	_, err := resolveTarget("gastown/crew/mel", ResolveTargetOptions{
		TownRoot: "/town",
		DryRun:   true,
	})
	if err == nil {
		t.Fatal("expected error in dry-run, got nil")
	}
	// Dry-run should fall through to the original opaque error path.
	if !strings.Contains(err.Error(), "resolving target") {
		t.Errorf("expected 'resolving target' in error, got: %v", err)
	}
}
