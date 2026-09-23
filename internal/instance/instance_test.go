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
// claims, reports ready and leaves when displaced; "handover" leaves a newer
// daemon in its place; "stay" never leaves; "exit" fails before claiming.
func helper(mode string) int {
	if mode == "exit" {
		return 3
	}

	claim, _, _ := Take(context.Background(), os.Getenv(claimEnv), time.Second)

	if mode == "handover" {
		if handOver() != nil {
			return 4
		}
	} else {
		claim.Ready()
	}

	for mode == "stay" || !claim.Taken() {
		time.Sleep(20 * time.Millisecond)
	}

	return 0
}

// handOver starts a daemon instance beside this one, which is what a second
// restart does: it claims the session, and this one then leaves without ever
// having read it.
func handOver() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	if err := os.Setenv(helperEnv, "daemon"); err != nil {
		return err
	}

	_, err = start(exe)

	return err
}

// take claims path for this process, which no test needs to see fail.
func take(t *testing.T, path string) *Claim {
	t.Helper()

	claim, _, err := Take(context.Background(), path, time.Second)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}

	return claim
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

	if !waitUntil(context.Background(), 5*time.Second, func() bool { return !alive(pid) }) {
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
	t.Parallel()

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
	t.Parallel()

	claim, stayed, err := Take(context.Background(), claimFile(t), time.Second)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}

	if stayed != 0 {
		t.Errorf("pid %d stayed, want nobody displaced", stayed)
	}

	if claim.Taken() {
		t.Error("a fresh claim reads as taken")
	}
}

func TestAClaimByAnotherProcessIsSeen(t *testing.T) {
	t.Parallel()

	path := claimFile(t)

	claim := take(t, path)
	// What a newer instance leaves behind, whether or not it is polling yet.
	writeRecord(t, path, record{PID: os.Getpid() + 1})

	if !claim.Taken() {
		t.Error("another process's claim was not seen")
	}
}

func TestAnUnreadableClaimDecidesNothing(t *testing.T) {
	t.Parallel()

	// A claim being written by another process can read as empty or as half a
	// line for a moment. That is not a takeover; the next look decides.
	path := claimFile(t)

	claim := take(t, path)

	if err := os.WriteFile(path, []byte(`{"pid": 4`), 0o600); err != nil {
		t.Fatal(err)
	}

	if claim.Taken() {
		t.Error("half a claim read as a takeover")
	}
}

func TestAMissingClaimIsNobodys(t *testing.T) {
	t.Parallel()

	// A user can empty the directory. That is not a takeover, and nothing
	// writes a claim outside Take: only a newer claim ends a run.
	path := claimFile(t)

	claim := take(t, path)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if claim.Taken() {
		t.Error("a missing claim read as a takeover")
	}
}

func TestAClaimWithNoPathIsInert(t *testing.T) {
	t.Parallel()

	// A user with no configuration directory still gets the plugin, only
	// without takeover: nothing here may fail the start.
	claim, stayed, err := Take(context.Background(), "", time.Second)
	if err == nil {
		t.Error("Take with no path reported no error to log")
	}

	if stayed != 0 {
		t.Errorf("pid %d stayed, want nothing waited for without a claim", stayed)
	}

	if claim.Taken() {
		t.Error("an inert claim reads as taken")
	}

	claim.Ready()
}

//nolint:paralleltest // the instance started inherits the environment
func TestAStaleClaimOfAnExitedProcessDisplacesNobody(t *testing.T) {
	// An instance that crashed leaves its claim behind; waiting on a pid that
	// is gone would only delay the first poll.
	path := claimFile(t)
	pid := instance(t, "stay", path)

	eventually(t, "the instance's claim", func() bool { return holder(path) == pid })
	end(t, pid)

	if _, stayed, _ := Take(context.Background(), path, time.Second); stayed != 0 {
		t.Errorf("pid %d stayed, want none for a process that exited", stayed)
	}
}

//nolint:paralleltest // the instance started inherits the environment
func TestAnInstanceLeavingKeepsItsSuccessorsClaim(t *testing.T) {
	// An instance leaves because a newer one claimed. Removing the claim on
	// the way out would remove that successor's, and the next instance would
	// then displace nobody.
	path := claimFile(t)
	old := instance(t, "daemon", path)

	eventually(t, "the old instance polling", func() bool { return marked(path, old) })

	take(t, path)
	eventually(t, "the old instance leaving", func() bool { return !alive(old) })

	if rec, ok := read(path); !ok || rec.PID != os.Getpid() {
		t.Errorf("claim = %+v after the old instance left, want this process's", rec)
	}
}

func TestReadyTouchesOnlyTheMarker(t *testing.T) {
	t.Parallel()

	// The first poll can outlast a takeover, and marking ready must not hand
	// the session back to the instance that was told to leave: the claim is
	// never written after it was taken, ready goes in a file of its own.
	path := claimFile(t)

	claim := take(t, path)
	writeRecord(t, path, record{PID: os.Getpid() + 1})
	claim.Ready()

	if rec, _ := read(path); rec.PID != os.Getpid()+1 {
		t.Errorf("claim holder = %d after ready, want the newer instance kept", rec.PID)
	}

	if rec, ok := read(marker(path)); !ok || rec.PID != os.Getpid() {
		t.Errorf("marker = %+v, want this process marked ready", rec)
	}
}

func TestReadyIsSeenOnlyForTheProcessThatMarkedIt(t *testing.T) {
	t.Parallel()

	// A marker left by an instance that has gone says nothing about the one
	// holding the claim now.
	path := claimFile(t)

	claim := take(t, path)
	claim.Ready()

	if !marked(path, os.Getpid()) {
		t.Fatal("a process that marked ready does not read as ready")
	}

	writeRecord(t, marker(path), record{PID: os.Getpid() + 1})

	if marked(path, os.Getpid()) {
		t.Error("another process's marker read as this one's readiness")
	}
}

//nolint:paralleltest // the instance started inherits the environment
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

//nolint:paralleltest // the instance started inherits the environment
func TestTakeReturnsOnceTheOldInstanceLeaves(t *testing.T) {
	path := claimFile(t)
	old := instance(t, "daemon", path)

	eventually(t, "the old instance polling", func() bool { return marked(path, old) })

	_, stayed, err := Take(context.Background(), path, 5*time.Second)
	if err != nil {
		t.Fatalf("Take: %v", err)
	}

	if stayed != 0 {
		t.Errorf("pid %d stayed, want the old instance gone", stayed)
	}

	if alive(old) {
		t.Error("Take returned while the old instance was alive")
	}
}

//nolint:paralleltest // the instance started inherits the environment
func TestTakeReportsAnInstanceThatStays(t *testing.T) {
	// A version that knew nothing of claims never leaves, and is never
	// killed: the new instance runs beside it and the user is told.
	path := claimFile(t)
	old := instance(t, "stay", path)

	eventually(t, "the old instance polling", func() bool { return marked(path, old) })

	if _, stayed, _ := Take(context.Background(), path, 200*time.Millisecond); stayed != old {
		t.Errorf("stayed = %d, want the instance %d that did not leave", stayed, old)
	}
}
