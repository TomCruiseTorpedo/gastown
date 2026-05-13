package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	worktreeintegrity "github.com/steveyegge/gastown/internal/worktree"
)

func TestEnsureRoleWorktreeIntegrityRequiresPolecatMetadata(t *testing.T) {
	townRoot := t.TempDir()
	cwd := filepath.Join(townRoot, "gastown", "polecats", "deathclaw", "gastown")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatal(err)
	}

	err := ensureRoleWorktreeIntegrity(cwd, townRoot, RolePolecat)
	if !errors.Is(err, worktreeintegrity.ErrIntegrityViolation) {
		t.Fatalf("ensureRoleWorktreeIntegrity() error = %v, want ErrIntegrityViolation", err)
	}
	if !strings.Contains(err.Error(), "gt doctor --fix") {
		t.Fatalf("ensureRoleWorktreeIntegrity() error = %v, want remediation", err)
	}
}

func TestEnsureRoleWorktreeIntegrityAllowsNeutralDirectoryWithoutMetadata(t *testing.T) {
	townRoot := t.TempDir()

	if err := ensureRoleWorktreeIntegrity(townRoot, townRoot, RoleUnknown); err != nil {
		t.Fatalf("ensureRoleWorktreeIntegrity() error = %v, want nil", err)
	}
}

func TestEnsureRoleWorktreeIntegrityRejectsMalformedOptionalMetadata(t *testing.T) {
	townRoot := t.TempDir()
	cwd := filepath.Join(townRoot, "scratch")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".git"), []byte("corrupted\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := ensureRoleWorktreeIntegrity(cwd, townRoot, RoleUnknown)
	if !errors.Is(err, worktreeintegrity.ErrIntegrityViolation) {
		t.Fatalf("ensureRoleWorktreeIntegrity() error = %v, want ErrIntegrityViolation", err)
	}
}
