package labels_test

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/herdr/herdrtest"
	"github.com/kryptamine/herdr-auto-title/internal/labels"
	"github.com/kryptamine/herdr-auto-title/internal/resolver"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

const theTab = "wE:t1"

func load(path string) *labels.Labels {
	return labels.Load(path, slog.New(slog.DiscardHandler))
}

// session is a stubbed Herdr holding one tab that carries label.
func session(label string) *herdrtest.Client {
	return herdrtest.New([]herdr.TabInfo{{TabID: theTab, Label: label}}, nil)
}

// name runs one whole pass over the session's only tab, wanting it named want.
func name(t *testing.T, l *labels.Labels, client *herdrtest.Client, want string) {
	t.Helper()

	pass, tab := open(t, l, client)
	pass.Tab(context.Background(), tab, resolver.Decision{Name: want})
	pass.Settle()
}

// open opens a pass over the session as it is now, and reads its only tab.
func open(
	t *testing.T,
	l *labels.Labels,
	client *herdrtest.Client,
) (*labels.Pass, state.TabState) {
	t.Helper()

	snapshot, err := herdr.SessionSnapshot(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}

	var tab state.TabState
	if len(snapshot.Tabs) > 0 {
		tab = state.TabFrom(snapshot.Tabs[0], "", 1, nil, false)
	}

	return l.Pass(client, snapshot), tab
}

func wantRenames(t *testing.T, client *herdrtest.Client, want ...string) {
	t.Helper()

	renames := client.Renames()
	if len(renames) != len(want) {
		t.Fatalf("renames = %v, want %v", renames, want)
	}

	for i, rename := range renames {
		if rename.Label != want[i] {
			t.Errorf("rename %d = %q, want %q", i, rename.Label, want[i])
		}
	}
}

func TestATabIsRenamedOnlyWhileItsNameDiffers(t *testing.T) {
	t.Parallel()

	l, client := load(""), session("1")

	name(t, l, client, "dashboard")
	name(t, l, client, "dashboard")
	name(t, l, client, "api")

	wantRenames(t, client, "dashboard", "api")
}

func TestATabTheUserRenamedIsLeftAlone(t *testing.T) {
	t.Parallel()

	l, client := load(""), session("1")
	name(t, l, client, "dashboard")

	client.SetTab(herdr.TabInfo{TabID: theTab, Label: "Important work"})
	name(t, l, client, "api")
	name(t, l, client, "api")

	wantRenames(t, client, "dashboard")

	if pass, _ := open(t, l, client); !pass.TabLocked(theTab) {
		t.Error("the tab the user renamed is not locked")
	}
}

// The claim is released as the pass opens, so the tab cleared by its owner is
// worth reading again on the very poll that finds it cleared.
func TestAClearedTabIsUnlockedBeforeAnythingIsAskedOfThePass(t *testing.T) {
	t.Parallel()

	l, client := load(""), session("1")
	name(t, l, client, "dashboard")

	client.SetTab(herdr.TabInfo{TabID: theTab, Label: "Important work"})
	name(t, l, client, "dashboard")

	client.SetTab(herdr.TabInfo{TabID: theTab, Label: ""})

	pass, tab := open(t, l, client)
	if pass.TabLocked(theTab) {
		t.Fatal("the tab is still locked after its owner cleared the name")
	}

	pass.Tab(context.Background(), tab, resolver.Decision{Name: "api"})

	wantRenames(t, client, "dashboard", "api")
}

// A stalled Herdr applies a rename after its call gave up. The label is this
// plugin's all the same, and read as the user's it would lock the tab for good.
func TestARenameThatGotNoAnswerIsNotTheUsersWhenItLands(t *testing.T) {
	t.Parallel()

	l, client := load(""), session("1")
	name(t, l, client, "dashboard")

	client.SetRenameError(herdr.ErrUnanswered)
	name(t, l, client, "api")

	client.SetRenameError(nil)
	name(t, l, client, "billing")

	wantRenames(t, client, "dashboard", "api", "billing")
}

// A rename Herdr refused never lands, so its label turning up later is
// somebody else's doing.
func TestARefusedRenameLeavesItsLabelToTheUser(t *testing.T) {
	t.Parallel()

	l, client := load(""), session("1")
	name(t, l, client, "dashboard")

	client.SetRenameError(errors.New("refused"))
	name(t, l, client, "api")

	client.SetRenameError(nil)
	client.SetTab(herdr.TabInfo{TabID: theTab, Label: "api"})
	name(t, l, client, "billing")

	if pass, _ := open(t, l, client); !pass.TabLocked(theTab) {
		t.Error("a label only the user can have written was not claimed")
	}
}

// A tab that closed under its rename never wore the name, so a tab turning up
// under its id and wearing that name was named by whoever made it.
func TestARenameOfAClosedTabIsNotCountedAsMade(t *testing.T) {
	t.Parallel()

	l, client := load(""), session("1")

	pass, tab := open(t, l, client)
	client.CloseTab(theTab)
	pass.Tab(context.Background(), tab, resolver.Decision{Name: "dashboard"})
	pass.Settle()

	client.SetTab(herdr.TabInfo{TabID: theTab, Label: "dashboard"})
	name(t, l, client, "api")

	wantRenames(t, client)

	if pass, _ := open(t, l, client); !pass.TabLocked(theTab) {
		t.Error("a name the plugin never managed to set was read as its own")
	}
}

// A pass cut short has not seen every tab, so the one after it is still the
// first: a tab it finds already named was named before the plugin looked.
func TestAPassThatNeverSettledLeavesTheNextOneTheFirst(t *testing.T) {
	t.Parallel()

	l, client := load(""), session("Important work")

	_, _ = open(t, l, client)

	name(t, l, client, "dashboard")

	wantRenames(t, client, "dashboard")
}

func TestAClaimIsKeptWhereTheNextRunFindsIt(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manual-names.json")

	l, client := load(path), session("1")
	name(t, l, client, "dashboard")

	client.SetTab(herdr.TabInfo{TabID: theTab, Label: "Important work"})
	name(t, l, client, "api")

	if pass, _ := open(t, load(path), client); !pass.TabLocked(theTab) {
		t.Error("the claim did not survive a reload")
	}
}
