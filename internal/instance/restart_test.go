package instance

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRestartHandsTheSessionToAFreshInstance(t *testing.T) {
	path := claimFile(t)
	old := instance(t, "daemon", path)

	eventually(t, "the old instance polling", func() bool { return marked(path, old) })

	pid, err := Restart(context.Background(), path, executable(t), 10*time.Second)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}

	t.Cleanup(func() { end(t, pid) })

	if pid == old {
		t.Fatalf("restart reported the old instance %d as the new one", pid)
	}

	if alive(old) {
		t.Error("the old instance is still running after the restart returned")
	}

	if err := pending(path, pid, 0); err != nil {
		t.Errorf("the new instance %d is not polling: %v", pid, err)
	}
}

func TestRestartStartsAnInstanceWhereNoneWasRunning(t *testing.T) {
	// The first start after an install or a link, which Herdr does not do.
	path := claimFile(t)
	asInstance(t, "daemon", path)

	pid, err := Restart(context.Background(), path, executable(t), 10*time.Second)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}

	t.Cleanup(func() { end(t, pid) })

	if holder(path) != pid {
		t.Errorf("no instance %d holding the claim after the restart", pid)
	}
}

func TestRestartReportsAnInstanceThatExitsBeforePolling(t *testing.T) {
	path := claimFile(t)
	asInstance(t, "exit", path)

	_, err := Restart(context.Background(), path, executable(t), 10*time.Second)
	if err == nil {
		t.Fatal("restart succeeded although the new instance exited")
	}

	if !strings.Contains(err.Error(), "exit") {
		t.Errorf("error %q does not say the instance exited", err)
	}
}

func TestRestartReportsAnOldInstanceThatStays(t *testing.T) {
	// The first upgrade from a version that knew nothing of claims: the new
	// instance polls, but the old one is still there naming tabs beside it.
	path := claimFile(t)
	old := instance(t, "stay", path)

	eventually(t, "the old instance polling", func() bool { return marked(path, old) })

	pid, err := Restart(context.Background(), path, executable(t), 4*time.Second)
	if pid != 0 {
		t.Cleanup(func() { end(t, pid) })
	} else if newer := holder(path); newer != old {
		t.Cleanup(func() { end(t, newer) })
	}

	if err == nil {
		t.Fatal("restart succeeded although the old instance stayed")
	}

	if !strings.Contains(err.Error(), strconv.Itoa(old)) {
		t.Errorf("error %q does not name the old instance %d", err, old)
	}
}

func TestRestartRefusesToStartAnInstanceItCannotWaitFor(t *testing.T) {
	// No configuration directory means no claim: the running instance would
	// never leave, and nothing would say when the new one is polling.
	asInstance(t, "stay", claimFile(t))

	if _, err := Restart(context.Background(), "", executable(t), time.Second); err == nil {
		t.Fatal("restart started an instance with nowhere to claim the session")
	}
}

func TestRestartRespectsItsContext(t *testing.T) {
	path := claimFile(t)
	asInstance(t, "stay", path)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Restart(ctx, path, executable(t), 10*time.Second)
	if err == nil {
		t.Fatal("restart succeeded with a cancelled context")
	}

	if newer := holder(path); newer != 0 {
		end(t, newer)
	}
}
