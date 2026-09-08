//go:build !windows

package herdr

import (
	"net"
	"os"
	"testing"
)

func TestAServerIsToldByTheSocketFileItBound(t *testing.T) {
	ln, path := listen(t)

	before := serverIdentity(path)
	if before == "" {
		t.Fatal("no identity for a bound socket")
	}

	if again := serverIdentity(path); again != before {
		t.Errorf("identity = %q on a second read, want %q", again, before)
	}

	// The successor binds the path anew. The old file is kept aside rather
	// than removed, so the two identities cannot share a recycled inode.
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	ln.close()

	if got := serverIdentity(path); got != "" {
		t.Errorf("identity = %q with the socket gone, want none", got)
	}

	successor, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen again on %s: %v", path, err)
	}

	defer successor.Close()

	if after := serverIdentity(path); after == "" || after == before {
		t.Errorf("identity = %q after a rebind, want one other than %q", after, before)
	}
}
