package witness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/lock"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	deaconNoticeMu               sync.Mutex
	deaconRecoveryNoticeCooldown = 5 * time.Minute
	sendWitnessMail              = func(router *mail.Router, msg *mail.Message) error { return router.Send(msg) }
	nudgeDeaconSession           = func(townRoot, message string) error {
		t := tmux.NewTmux()
		return t.NudgeSessionWithOpts(session.DeaconSessionName(), message, tmux.NudgeOpts{TownRoot: townRoot})
	}
)

type deaconNoticeState struct {
	LastSent map[string]time.Time `json:"last_sent"`
}

func deaconNoticeStateFile(townRoot string) string {
	return filepath.Join(townRoot, "witness", "deacon-notifications.json")
}

func loadDeaconNoticeState(townRoot string) *deaconNoticeState {
	data, err := os.ReadFile(deaconNoticeStateFile(townRoot)) //nolint:gosec // G304: path from trusted townRoot
	if err != nil {
		return &deaconNoticeState{LastSent: make(map[string]time.Time)}
	}
	var state deaconNoticeState
	if err := json.Unmarshal(data, &state); err != nil {
		return &deaconNoticeState{LastSent: make(map[string]time.Time)}
	}
	if state.LastSent == nil {
		state.LastSent = make(map[string]time.Time)
	}
	return &state
}

func saveDeaconNoticeState(townRoot string, state *deaconNoticeState) error {
	stateFile := deaconNoticeStateFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return fmt.Errorf("creating witness dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling deacon notice state: %w", err)
	}
	return os.WriteFile(stateFile, data, 0600)
}

func shouldThrottleDeaconNotice(workDir, key string, cooldown time.Duration) bool {
	if key == "" || cooldown <= 0 {
		return false
	}

	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}

	deaconNoticeMu.Lock()
	defer deaconNoticeMu.Unlock()

	unlock, flockErr := lock.FlockAcquire(deaconNoticeStateFile(townRoot) + ".flock")
	if flockErr == nil {
		defer unlock()
	}

	now := time.Now().UTC()
	state := loadDeaconNoticeState(townRoot)
	if last, ok := state.LastSent[key]; ok && now.Sub(last) < cooldown {
		return true
	}
	state.LastSent[key] = now
	_ = saveDeaconNoticeState(townRoot, state) // Best-effort: throttle failure must not block recovery.
	return false
}

func notifyDeaconRecoveredBead(workDir, rigName, hookBead, polecatName, status string, respawnCount, maxRespawns int, router *mail.Router) {
	if router == nil {
		return
	}

	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}

	if respawnCount >= maxRespawns {
		subject := fmt.Sprintf("SPAWN_STORM RECOVERED_BEAD %s (respawned %dx)", hookBead, respawnCount)
		body := fmt.Sprintf(`Recovered abandoned bead from dead polecat.

Bead: %s
Polecat: %s/%s
Previous Status: %s
Respawn Count: %d

SPAWN STORM: bead has been reset %d times. Next respawn will be BLOCKED.
Check polecat completion protocol or close the bead manually.

The bead has been reset to open with no assignee.
Please re-dispatch to an available polecat.`, hookBead, rigName, polecatName, status, respawnCount, respawnCount)
		msg := &mail.Message{
			From:     fmt.Sprintf("%s/witness", rigName),
			To:       "deacon/",
			Subject:  subject,
			Priority: mail.PriorityUrgent,
			Type:     mail.TypeTask,
			Body:     body,
		}
		if err := sendWitnessMail(router, msg); err != nil {
			fmt.Fprintf(os.Stderr, "witness: failed to send SPAWN_STORM RECOVERED_BEAD mail for %s: %v, attempting nudge fallback\n", hookBead, err)
			nudgeMsg := fmt.Sprintf("SPAWN_STORM RECOVERED_BEAD %s from %s/%s (status=%s, respawns=%d) — mail send failed, please re-dispatch or close manually",
				hookBead, rigName, polecatName, status, respawnCount)
			if nudgeErr := nudgeDeaconSession(townRoot, nudgeMsg); nudgeErr != nil {
				fmt.Fprintf(os.Stderr, "witness: nudge fallback to deacon also failed for %s: %v\n", hookBead, nudgeErr)
			}
		}
		return
	}

	// The open bead is the durable source of truth for routine re-dispatch. Avoid
	// permanent mail commits and wake Deacon at most once per rig/cooldown window.
	key := fmt.Sprintf("recovered-bead:%s", rigName)
	if shouldThrottleDeaconNotice(workDir, key, deaconRecoveryNoticeCooldown) {
		return
	}
	nudgeMsg := fmt.Sprintf("RECOVERED_BEAD: witness reset abandoned bead(s) in %s; latest=%s from %s/%s (status=%s, respawns=%d). Run `bd ready` or patrol to re-dispatch.",
		rigName, hookBead, rigName, polecatName, status, respawnCount)
	if err := nudgeDeaconSession(townRoot, nudgeMsg); err != nil {
		fmt.Fprintf(os.Stderr, "witness: routine RECOVERED_BEAD nudge to deacon failed for %s: %v\n", hookBead, err)
	}
}
