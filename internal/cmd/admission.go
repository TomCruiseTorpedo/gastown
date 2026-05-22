package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	gtlock "github.com/steveyegge/gastown/internal/lock"
)

var ErrPolecatAdmissionDenied = errors.New("polecat admission denied")

var countWorkingPolecatsForAdmission = countWorkingPolecats

type admissionReservation struct {
	once    sync.Once
	release func()
}

func (r *admissionReservation) Release() {
	if r == nil || r.release == nil {
		return
	}
	r.once.Do(r.release)
}

func reservePolecatAdmission(townRoot string, maxPolecats int) (*admissionReservation, error) {
	if townRoot == "" || maxPolecats <= 0 {
		return &admissionReservation{}, nil
	}

	lockDir := filepath.Join(townRoot, ".runtime", "locks", "admission")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("creating admission lock dir: %w", err)
	}

	globalRelease, err := gtlock.FlockAcquire(filepath.Join(lockDir, "polecat-admission.flock"))
	if err != nil {
		return nil, fmt.Errorf("acquiring admission lock: %w", err)
	}
	defer globalRelease()

	working := countWorkingPolecatsForAdmission()
	activeReservations, err := countAdmissionReservations(lockDir, maxPolecats)
	if err != nil {
		return nil, err
	}
	if working+activeReservations >= maxPolecats {
		return nil, fmt.Errorf("%w: %d active/reserved polecat slots (max %d)", ErrPolecatAdmissionDenied, working+activeReservations, maxPolecats)
	}

	for i := 0; i < maxPolecats; i++ {
		release, locked, err := gtlock.FlockTryAcquire(filepath.Join(lockDir, fmt.Sprintf("slot-%d.flock", i)))
		if err != nil {
			return nil, fmt.Errorf("acquiring admission reservation slot: %w", err)
		}
		if locked {
			return &admissionReservation{release: release}, nil
		}
	}

	return nil, fmt.Errorf("%w: no reservation slot available", ErrPolecatAdmissionDenied)
}

func countAdmissionReservations(lockDir string, maxPolecats int) (int, error) {
	active := 0
	for i := 0; i < maxPolecats; i++ {
		release, locked, err := gtlock.FlockTryAcquire(filepath.Join(lockDir, fmt.Sprintf("slot-%d.flock", i)))
		if err != nil {
			return 0, fmt.Errorf("checking admission reservation slot: %w", err)
		}
		if locked {
			release()
			continue
		}
		active++
	}
	return active, nil
}

func isAdmissionDenial(err error) bool {
	return errors.Is(err, ErrPolecatAdmissionDenied)
}
