package instance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the process under test: started with helperEnv
// set it behaves as an instance would, claiming the file claimEnv names.
const (
	helperEnv = "HERDR_AUTO_TITLE_TEST_INSTANCE"
	claimEnv  = "HERDR_AUTO_TITLE_TEST_CLAIM"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		os.Exit(helper(mode))
	}

	os.Exit(m.Run())
}

// helper is what the test binary does when started as an instance: "daemon"
// claims, reports ready and leaves when displaced; "stay" never leaves, like a
// version that knew nothing of claims; "exit" fails before claiming anything.
func helper(mode string) int {
	if mode == "exit" {
		return 3
	}

	claim, _ := Take(os.Getenv(claimEnv))
	claim.AwaitDisplaced(context.Background(), time.Second)
	claim.Ready()

	for mode == "stay" || !claim.Taken() {
		time.Sleep(20 * time.Millisecond)
	}

	claim.Release()

	return 0
}

func claimFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "claim.json")
}

// asInstance makes the test binary behave as an instance of the given mode
// when started next, whether by a test or by a restart.
func asInstance(t *testing.T, mode, path string) {
	t.Helper()
	t.Setenv(helperEnv, mode)
	t.Setenv(claimEnv, path)
}

func executable(t *testing.T) string {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	return exe
}

// instance starts the test binary as an instance of the given mode, and makes
// sure it is gone when the test ends.
func instance(t *testing.T, mode, path string) int {
	t.Helper()
	asInstance(t, mode, path)

	proc, err := start(executable(t))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Reaped as it exits, or alive would go on seeing it.
	go func() { _, _ = proc.Wait() }()

	t.Cleanup(func() { end(t, proc.Pid) })

	return proc.Pid
}

// end kills a process the test started and waits until it is gone, so a test
// asserting on alive never races the kill.
func end(t *testing.T, pid int) {
	t.Helper()

	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}

	if !await(context.Background(), pid, 5*time.Second) {
		t.Fatalf("pid %d did not end", pid)
	}
}

// eventually polls cond until it holds or two seconds pass.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within two seconds", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func writeRecord(t *testing.T, path string, rec record) {
	t.Helper()

	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAClaimIsNamedAfterItsSocket(t *testing.T) {
	// One session, one socket, one claim: two sessions of the same user must
	// not displace each other, and a path is not a file name.
	dir := t.TempDir()

	a := File(dir, `/home/dev/.config/herdr/herdr.sock`)
	b := File(dir, `/home/dev/.config/herdr/sessions/work/herdr.sock`)

	if a == b {
		t.Errorf("two sockets share the claim %q", a)
	}

	if a != File(dir, `/home/dev/.config/herdr/herdr.sock`) {
		t.Errorf("the same socket named two claims")
	}

	if !strings.HasPrefix(a, dir) {
		t.Errorf("claim %q is not under %q", a, dir)
	}

	if got := File("", "/tmp/herdr.sock"); got != "" {
		t.Errorf("File with no directory = %q, want none", got)
	}

	if got := marker(a); got == a || filepath.Dir(got) != filepath.Dir(a) {
		t.Errorf("marker %q, want a file of its own beside the claim %q", got, a)
	}
}

func TestTheFirstClaimDisplacesNobody(t *testing.T) {
	claim, err := Take(claimFile(t))
	if err != nil {
		t.Fatalf("Take: %v", err)
	}

	if pid := claim.Displaced(); pid != 0 {
		t.Errorf("displaced pid %d, want none", pid)
	}

	if claim.Taken() {
		t.Error("a fresh claim reads as taken")
	}

	if !claim.AwaitDisplaced(context.Background(), time.Second) {
		t.Error("waiting for nobody did not return at once")
	}
}

func TestAClaimByAnotherProcessIsSeen(t *testing.T) {
	path := claimFile(t)

	claim, _ := Take(path)
	// What a newer instance leaves behind, whether or not it is polling yet.
	writeRecord(t, path, record{PID: os.Getpid() + 1})

	if !claim.Taken() {
		t.Error("another process's claim was not seen")
	}
}

func TestAnUnreadableClaimDecidesNothing(t *testing.T) {
	// A claim being written by another process can read as empty or as half a
	// line for a moment. That is not a takeover; the next look decides.
	path := claimFile(t)

	claim, _ := Take(path)

	if err := os.WriteFile(path, []byte(`{"pid": 4`), 0o600); err != nil {
		t.Fatal(err)
	}

	if claim.Taken() {
		t.Error("half a claim read as a takeover")
	}
}

func TestAMissingClaimIsTakenAgain(t *testing.T) {
	// An instance leaving can remove a claim taken from it a moment before,
	// and a user can empty the directory; the holder puts its claim back.
	path := claimFile(t)

	claim, _ := Take(path)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if claim.Taken() {
		t.Error("a missing claim read as a takeover")
	}

	if got := holder(path); got != os.Getpid() {
		t.Errorf("claim holder = %d after the file went missing, want it taken again", got)
	}
}

func TestAClaimWithNoPathIsInert(t *testing.T) {
	// A user with no configuration directory still gets the plugin, only
	// without takeover: nothing here may fail the start.
	claim, err := Take("")
	if err == nil {
		t.Error("Take with no path reported no error to log")
	}

	if claim.Taken() {
		t.Error("an inert claim reads as taken")
	}

	claim.Ready()
	claim.Release()
}

func TestAStaleClaimOfAnExitedProcessDisplacesNobody(t *testing.T) {
	// An instance that crashed leaves its claim behind; waiting on a pid that
	// is gone would only delay the first poll.
	path := claimFile(t)
	pid := instance(t, "stay", path)

	eventually(t, "the instance's claim", func() bool { return holder(path) == pid })
	end(t, pid)

	claim, _ := Take(path)
	if got := claim.Displaced(); got != 0 {
		t.Errorf("displaced pid %d, want none for a process that exited", got)
	}
}

func TestAReleasedClaimIsGone(t *testing.T) {
	path := claimFile(t)

	claim, _ := Take(path)
	claim.Release()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("claim still there after release: %v", err)
	}
}

func TestReleaseLeavesANewerClaimAlone(t *testing.T) {
	// The old instance leaves because a newer one claimed; taking the claim
	// with it would leave the newer one unprotected from the next.
	path := claimFile(t)

	claim, _ := Take(path)
	writeRecord(t, path, record{PID: os.Getpid() + 1})
	claim.Release()

	if _, ok := read(path); !ok {
		t.Error("the newer instance's claim was removed with the old one's release")
	}
}

func TestReadyTouchesOnlyTheMarker(t *testing.T) {
	// The first poll can outlast a takeover, and marking ready must not hand
	// the session back to the instance that was told to leave: the claim is
	// never written after it was taken, ready goes in a file of its own.
	path := claimFile(t)

	claim, _ := Take(path)
	writeRecord(t, path, record{PID: os.Getpid() + 1})
	claim.Ready()

	if rec, _ := read(path); rec.PID != os.Getpid()+1 {
		t.Errorf("claim holder = %d after ready, want the newer instance kept", rec.PID)
	}

	if rec, ok := read(marker(path)); !ok || rec.PID != os.Getpid() {
		t.Errorf("marker = %+v, want this process marked ready", rec)
	}
}

func TestReadyIsSeenOnlyForTheClaimsHolder(t *testing.T) {
	// A marker left by an instance that has gone says nothing about the one
	// holding the claim now.
	path := claimFile(t)

	claim, _ := Take(path)
	claim.Ready()

	if !ready(path) {
		t.Fatal("a claim holder that marked ready does not read as ready")
	}

	writeRecord(t, marker(path), record{PID: os.Getpid() + 1})

	if ready(path) {
		t.Error("another process's marker read as this holder's readiness")
	}
}

func TestAliveTellsARunningProcessFromOneThatExited(t *testing.T) {
	pid := instance(t, "stay", claimFile(t))

	if !alive(pid) {
		t.Fatalf("pid %d reads as gone while running", pid)
	}

	end(t, pid)

	if alive(pid) {
		t.Errorf("pid %d reads as alive after exiting", pid)
	}
}

func TestAwaitDisplacedReturnsOnceTheOldInstanceLeaves(t *testing.T) {
	path := claimFile(t)
	old := instance(t, "daemon", path)

	eventually(t, "the old instance polling", func() bool { return ready(path) })

	claim, _ := Take(path)
	if got := claim.Displaced(); got != old {
		t.Fatalf("displaced pid %d, want %d", got, old)
	}

	if !claim.AwaitDisplaced(context.Background(), 5*time.Second) {
		t.Error("the old instance did not leave once displaced")
	}

	if alive(old) {
		t.Error("AwaitDisplaced returned while the old instance was alive")
	}
}

func TestAwaitDisplacedGivesUpOnAnInstanceThatStays(t *testing.T) {
	path := claimFile(t)
	instance(t, "stay", path)

	eventually(t, "the old instance polling", func() bool { return ready(path) })

	claim, _ := Take(path)
	if claim.AwaitDisplaced(context.Background(), 200*time.Millisecond) {
		t.Error("an instance that stays was reported gone")
	}
}
