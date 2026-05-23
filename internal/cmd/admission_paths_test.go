package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
)

func setupAdmissionPathTestTown(t *testing.T, maxPolecats int) string {
	t.Helper()

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "settings"), 0755); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown", "mayor", "rig", ".beads"), 0755); err != nil {
		t.Fatalf("mkdir gastown rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown", ".beads"), 0755); err != nil {
		t.Fatalf("mkdir gastown redirect: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "gastown", ".beads", "redirect"), []byte("mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write rig redirect: %v", err)
	}

	rigs := &config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			"gastown": {
				GitURL:  "git@github.com:test/gastown.git",
				AddedAt: time.Now().Truncate(time.Second),
				BeadsConfig: &config.BeadsConfig{
					Repo:   "local",
					Prefix: "gt-",
				},
			},
		},
	}
	if err := config.SaveRigsConfig(filepath.Join(townRoot, "mayor", "rigs.json"), rigs); err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}
	if err := beads.WriteRoutes(filepath.Join(townRoot, ".beads"), []beads.Route{{Prefix: "gt-", Path: "gastown/mayor/rig"}}); err != nil {
		t.Fatalf("write routes: %v", err)
	}
	configureScheduler(t, townRoot, maxPolecats, 10)

	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	bdScript := `#!/bin/sh
if [ "$1" = "--db" ]; then
  shift
  shift
fi
cmd="$1"
case "$cmd" in
  show)
    echo '[{"id":"gt-work","title":"Admission work","status":"open","assignee":"","description":""}]'
    ;;
  *)
    exit 0
    ;;
esac
`
	bdScriptWindows := `@echo off
set cmd=%1
if "%cmd%"=="--db" set cmd=%3
if "%cmd%"=="show" (
  echo [{"id":"gt-work","title":"Admission work","status":"open","assignee":"","description":""}]
  exit /b 0
)
exit /b 0
`
	writeBDStub(t, binDir, bdScript, bdScriptWindows)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GT_TEST_NO_NUDGE", "1")
	t.Setenv("GT_TEST_SKIP_HOOK_VERIFY", "1")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	return townRoot
}

func TestConfiguredAdmissionMaxReadsPositiveSchedulerCap(t *testing.T) {
	townRoot := setupAdmissionPathTestTown(t, 2)

	if got := configuredAdmissionMax(townRoot); got != 2 {
		t.Fatalf("configuredAdmissionMax() = %d, want 2", got)
	}

	configureScheduler(t, townRoot, 0, 10)
	if got := configuredAdmissionMax(townRoot); got != 0 {
		t.Fatalf("configuredAdmissionMax() with disabled scheduler = %d, want 0", got)
	}
}

func TestResolveTargetPassesAdmissionMaxToRigAndNamedFallback(t *testing.T) {
	townRoot := setupAdmissionPathTestTown(t, 2)

	prevSpawn := spawnPolecatForSling
	prevResolve := resolveTargetAgentFn
	t.Cleanup(func() {
		spawnPolecatForSling = prevSpawn
		resolveTargetAgentFn = prevResolve
	})

	var got []int
	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		got = append(got, opts.AdmissionMax)
		return nil, ErrPolecatAdmissionDenied
	}

	_, err := resolveTarget("gastown", ResolveTargetOptions{
		TownRoot:     townRoot,
		HookBead:     "gt-work",
		AdmissionMax: configuredAdmissionMax(townRoot),
	})
	if !errors.Is(err, ErrPolecatAdmissionDenied) {
		t.Fatalf("resolveTarget rig error = %v, want ErrPolecatAdmissionDenied", err)
	}

	resolveTargetAgentFn = func(string) (string, string, string, error) {
		return "", "", "", fmt.Errorf("session missing")
	}
	_, err = resolveTarget("gastown/polecats/toast", ResolveTargetOptions{
		TownRoot:     townRoot,
		HookBead:     "gt-work",
		AdmissionMax: configuredAdmissionMax(townRoot),
	})
	if !errors.Is(err, ErrPolecatAdmissionDenied) {
		t.Fatalf("resolveTarget fallback error = %v, want ErrPolecatAdmissionDenied", err)
	}

	if len(got) != 2 || got[0] != 2 || got[1] != 2 {
		t.Fatalf("AdmissionMax values = %v, want [2 2]", got)
	}
}

func TestStandaloneFormulaSlingPassesConfiguredAdmissionMax(t *testing.T) {
	townRoot := setupAdmissionPathTestTown(t, 3)

	prevSpawn := spawnPolecatForSling
	prevDryRun := slingDryRun
	prevCreate := slingCreate
	prevAccount := slingAccount
	prevAgent := slingAgent
	prevNoBoot := slingNoBoot
	t.Cleanup(func() {
		spawnPolecatForSling = prevSpawn
		slingDryRun = prevDryRun
		slingCreate = prevCreate
		slingAccount = prevAccount
		slingAgent = prevAgent
		slingNoBoot = prevNoBoot
	})
	slingDryRun = false
	slingCreate = false
	slingAccount = ""
	slingAgent = ""
	slingNoBoot = true

	got := 0
	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		got = opts.AdmissionMax
		return nil, ErrPolecatAdmissionDenied
	}

	err := runSlingFormula(context.Background(), []string{"mol-review", "gastown"})
	if !errors.Is(err, ErrPolecatAdmissionDenied) {
		t.Fatalf("runSlingFormula error = %v, want ErrPolecatAdmissionDenied", err)
	}
	if got != 3 {
		t.Fatalf("AdmissionMax = %d, want 3", got)
	}
	_ = townRoot
}

func TestExecuteSlingDefaultsToConfiguredAdmissionMax(t *testing.T) {
	townRoot := setupAdmissionPathTestTown(t, 4)

	prevSpawn := spawnPolecatForSling
	t.Cleanup(func() { spawnPolecatForSling = prevSpawn })

	got := 0
	spawnPolecatForSling = func(rigName string, opts SlingSpawnOptions) (*SpawnedPolecatInfo, error) {
		got = opts.AdmissionMax
		return nil, ErrPolecatAdmissionDenied
	}

	_, err := executeSling(SlingParams{
		BeadID:   "gt-work",
		RigName:  "gastown",
		TownRoot: townRoot,
		BeadsDir: filepath.Join(townRoot, ".beads"),
	})
	if !errors.Is(err, ErrPolecatAdmissionDenied) {
		t.Fatalf("executeSling error = %v, want ErrPolecatAdmissionDenied", err)
	}
	if got != 4 {
		t.Fatalf("AdmissionMax = %d, want 4", got)
	}
}

func TestReservePolecatAdmissionConcurrentAdmitsAtMostMax(t *testing.T) {
	townRoot := t.TempDir()
	prevCount := countPolecatSlotsForAdmission
	t.Cleanup(func() { countPolecatSlotsForAdmission = prevCount })
	countPolecatSlotsForAdmission = func(string) (int, error) { return 0, nil }

	const maxPolecats = 2
	const attempts = 10
	start := make(chan struct{})
	doneCh := make(chan error, attempts)
	var successes atomic.Int32
	var reservationsMu sync.Mutex
	var reservations []*admissionReservation

	for i := 0; i < attempts; i++ {
		go func() {
			<-start
			reservation, err := reservePolecatAdmission(townRoot, maxPolecats)
			if err != nil {
				if !errors.Is(err, ErrPolecatAdmissionDenied) {
					doneCh <- err
					return
				}
				doneCh <- nil
				return
			}
			successes.Add(1)
			reservationsMu.Lock()
			reservations = append(reservations, reservation)
			reservationsMu.Unlock()
			doneCh <- nil
		}()
	}
	close(start)

	timeout := time.After(5 * time.Second)
	for i := 0; i < attempts; i++ {
		select {
		case err := <-doneCh:
			if err != nil {
				t.Fatalf("reservePolecatAdmission unexpected error: %v", err)
			}
		case <-timeout:
			t.Fatal("timed out waiting for admission attempts")
		}
	}

	if got := int(successes.Load()); got != maxPolecats {
		t.Fatalf("successful reservations = %d, want %d", got, maxPolecats)
	}
	for _, reservation := range reservations {
		reservation.Release()
	}
}

func TestSchedulerAdmissionDenialLeavesContextQueued(t *testing.T) {
	fields := &capacity.SlingContextFields{DispatchFailures: 0}
	err := fmt.Errorf("sling failed: %w", ErrPolecatAdmissionDenied)

	if !handleAdmissionDeniedDispatch("gt-work", err) {
		t.Fatal("admission denial should be handled as queued capacity denial")
	}
	if fields.DispatchFailures != 0 {
		t.Fatalf("DispatchFailures = %d, want 0", fields.DispatchFailures)
	}
}
