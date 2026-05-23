package witness

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/mail"
)

func TestNotifyDeaconRecoveredBead_RoutineUsesRateLimitedNudge(t *testing.T) {
	oldSend := sendWitnessMail
	oldNudge := nudgeDeaconSession
	oldCooldown := deaconRecoveryNoticeCooldown
	deaconRecoveryNoticeCooldown = time.Hour
	t.Cleanup(func() {
		sendWitnessMail = oldSend
		nudgeDeaconSession = oldNudge
		deaconRecoveryNoticeCooldown = oldCooldown
	})

	mailCount := 0
	sendWitnessMail = func(router *mail.Router, msg *mail.Message) error {
		mailCount++
		return nil
	}
	var nudges []string
	nudgeDeaconSession = func(townRoot, message string) error {
		nudges = append(nudges, message)
		return nil
	}

	workDir := t.TempDir()
	router := mail.NewRouter(workDir)
	notifyDeaconRecoveredBead(workDir, "testrig", "gt-work1", "alpha", "hooked", 1, 3, router)
	notifyDeaconRecoveredBead(workDir, "testrig", "gt-work2", "bravo", "in_progress", 1, 3, router)

	if mailCount != 0 {
		t.Fatalf("routine recovered bead sent %d durable mail messages, want 0", mailCount)
	}
	if len(nudges) != 1 {
		t.Fatalf("nudges = %d, want 1 rate-limited nudge", len(nudges))
	}
	if !strings.Contains(nudges[0], "RECOVERED_BEAD") || !strings.Contains(nudges[0], "bd ready") {
		t.Fatalf("nudge %q does not explain recovered-bead redispatch", nudges[0])
	}
}

func TestNotifyDeaconRecoveredBead_SpawnStormUsesDurableMail(t *testing.T) {
	oldSend := sendWitnessMail
	oldNudge := nudgeDeaconSession
	t.Cleanup(func() {
		sendWitnessMail = oldSend
		nudgeDeaconSession = oldNudge
	})

	var sent *mail.Message
	sendWitnessMail = func(router *mail.Router, msg *mail.Message) error {
		copy := *msg
		sent = &copy
		return nil
	}
	nudgeCount := 0
	nudgeDeaconSession = func(townRoot, message string) error {
		nudgeCount++
		return nil
	}

	workDir := t.TempDir()
	notifyDeaconRecoveredBead(workDir, "testrig", "gt-work1", "alpha", "hooked", 3, 3, mail.NewRouter(workDir))

	if sent == nil {
		t.Fatal("spawn storm did not send durable mail")
	}
	if sent.To != "deacon/" {
		t.Fatalf("To = %q, want deacon/", sent.To)
	}
	if sent.Priority != mail.PriorityUrgent {
		t.Fatalf("Priority = %q, want urgent", sent.Priority)
	}
	if sent.Type != mail.TypeTask {
		t.Fatalf("Type = %q, want task", sent.Type)
	}
	if !strings.Contains(sent.Subject, "SPAWN_STORM RECOVERED_BEAD") {
		t.Fatalf("Subject = %q, want spawn storm recovered bead", sent.Subject)
	}
	if nudgeCount != 0 {
		t.Fatalf("nudge fallback called %d times despite successful mail", nudgeCount)
	}
}

func TestNotifyDeaconRecoveredBead_DurableMailFailureFallsBackToNudge(t *testing.T) {
	oldSend := sendWitnessMail
	oldNudge := nudgeDeaconSession
	t.Cleanup(func() {
		sendWitnessMail = oldSend
		nudgeDeaconSession = oldNudge
	})

	sendWitnessMail = func(router *mail.Router, msg *mail.Message) error {
		return errors.New("mail unavailable")
	}
	var fallback string
	nudgeDeaconSession = func(townRoot, message string) error {
		fallback = message
		return nil
	}

	workDir := t.TempDir()
	notifyDeaconRecoveredBead(workDir, "testrig", "gt-work1", "alpha", "hooked", 3, 3, mail.NewRouter(workDir))

	if !strings.Contains(fallback, "SPAWN_STORM RECOVERED_BEAD") {
		t.Fatalf("fallback nudge = %q, want spawn storm recovered bead", fallback)
	}
}
