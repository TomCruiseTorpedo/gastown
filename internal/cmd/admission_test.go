package cmd

import (
	"errors"
	"fmt"
	"testing"
)

func TestReservePolecatAdmissionHoldsSlotUntilRelease(t *testing.T) {
	townRoot := t.TempDir()
	prevCount := countWorkingPolecatsForAdmission
	t.Cleanup(func() { countWorkingPolecatsForAdmission = prevCount })
	countWorkingPolecatsForAdmission = func() int { return 0 }

	reservation, err := reservePolecatAdmission(townRoot, 1)
	if err != nil {
		t.Fatalf("first reservePolecatAdmission: %v", err)
	}

	second, err := reservePolecatAdmission(townRoot, 1)
	if err == nil {
		second.Release()
		t.Fatal("second reservePolecatAdmission succeeded while first reservation was held")
	}
	if !errors.Is(err, ErrPolecatAdmissionDenied) {
		t.Fatalf("second reservePolecatAdmission error = %v, want ErrPolecatAdmissionDenied", err)
	}

	reservation.Release()
	reservation.Release() // idempotent; rollback and success paths may both attempt release.

	third, err := reservePolecatAdmission(townRoot, 1)
	if err != nil {
		t.Fatalf("third reservePolecatAdmission after release: %v", err)
	}
	third.Release()
}

func TestReservePolecatAdmissionCountsDurableAssignments(t *testing.T) {
	townRoot := t.TempDir()
	prevCount := countWorkingPolecatsForAdmission
	t.Cleanup(func() { countWorkingPolecatsForAdmission = prevCount })
	countWorkingPolecatsForAdmission = func() int { return 1 }

	reservation, err := reservePolecatAdmission(townRoot, 1)
	if err == nil {
		reservation.Release()
		t.Fatal("reservePolecatAdmission succeeded despite full durable capacity")
	}
	if !errors.Is(err, ErrPolecatAdmissionDenied) {
		t.Fatalf("reservePolecatAdmission error = %v, want ErrPolecatAdmissionDenied", err)
	}
}

func TestAdmissionDenialRecognizesWrappedErrors(t *testing.T) {
	err := fmt.Errorf("sling failed: %w", fmt.Errorf("spawning polecat: %w", ErrPolecatAdmissionDenied))
	if !isAdmissionDenial(err) {
		t.Fatalf("isAdmissionDenial(%v) = false, want true", err)
	}
}
