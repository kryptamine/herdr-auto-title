package herdr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAServerIsToldByTheMarkerItWrote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "herdr.sock")

	if got := serverIdentity(path); got != "" {
		t.Errorf("identity = %q with no marker, want none", got)
	}

	write := func(marker string) {
		t.Helper()

		if err := os.WriteFile(path, []byte(marker), 0o600); err != nil {
			t.Fatalf("write marker: %v", err)
		}
	}

	write("4242:1700000000000000000")

	before := serverIdentity(path)
	if before != "4242:1700000000000000000" {
		t.Fatalf("identity = %q, want the marker as written", before)
	}

	write("4343:1700000000000000001")

	if after := serverIdentity(path); after == before {
		t.Errorf("identity = %q after a successor wrote its marker, want another", after)
	}
}
