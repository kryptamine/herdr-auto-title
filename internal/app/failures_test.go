package app

import "testing"

func TestARunOfFailuresIsLoggedOnABackoff(t *testing.T) {
	// Polls run twice a second, so an hour of Herdr being down is seven
	// thousand identical warnings unless the run is allowed to double.
	var failures failureLog

	var logged []int

	for range 2000 {
		if run := failures.failed(); run > 0 {
			logged = append(logged, run)
		}
	}

	want := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}
	if len(logged) != len(want) {
		t.Fatalf("logged %v, want %v", logged, want)
	}

	for i, run := range want {
		if logged[i] != run {
			t.Fatalf("logged %v, want %v", logged, want)
		}
	}

	if run := failures.recovered(); run != 2000 {
		t.Errorf("recovery reported %d missed polls, want 2000", run)
	}

	if run := failures.recovered(); run != 0 {
		t.Errorf("recovery reported %d after nothing went wrong, want 0", run)
	}
}
