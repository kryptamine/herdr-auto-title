package resolver

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func tabWithCWD(dir string) state.TabState {
	return state.TabState{
		ID: "wE:t1",
		Panes: []*state.PaneState{
			{ID: "wE:p1", Dir: dir, Focused: true},
		},
	}
}

func TestResolveFromCWD(t *testing.T) {
	home := t.TempDir()
	source := CWD{home: filepath.Clean(home)}
	r := New(DefaultMaxLength, source)

	tests := []struct {
		name       string
		cwd        string
		want       string
		wantReason string
	}{
		{"project directory becomes the title", "/Users/dev/work/dashboard", "dashboard", "cwd"},
		{
			"nested directory uses its own basename",
			"/Users/dev/work/dashboard/src/api",
			"api",
			"cwd",
		},
		{"trailing slash is ignored", "/Users/dev/work/dashboard/", "dashboard", "cwd"},
		{"home directory falls back", home, GenericFallback, "generic_fallback"},
		{"filesystem root falls back", "/", GenericFallback, "generic_fallback"},
		{"relative path falls back", "work/dashboard", GenericFallback, "generic_fallback"},
		{"empty path falls back", "", GenericFallback, "generic_fallback"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := r.Resolve(tabWithCWD(tc.cwd))
			if got.Name != tc.want {
				t.Errorf("name = %q, want %q", got.Name, tc.want)
			}

			if got.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
	}
}

func TestResolveNamesATabAfterItsDirectory(t *testing.T) {
	r := New(DefaultMaxLength, NewCWD())
	tab := state.TabState{
		ID: "wE:t1",
		Panes: []*state.PaneState{
			{ID: "wE:p1", Dir: "/Users/dev/work/api", Focused: true},
		},
	}

	if got := r.Resolve(tab); got.Name != "api" {
		t.Errorf("name = %q, want %q", got.Name, "api")
	}
}

func TestResolveTabWithoutPanes(t *testing.T) {
	r := New(DefaultMaxLength, NewCWD())

	got := r.Resolve(state.TabState{ID: "wE:t1"})
	if got.Name != GenericFallback {
		t.Errorf("name = %q, want %q", got.Name, GenericFallback)
	}
}

func TestResolveTruncatesToMaxLength(t *testing.T) {
	long := strings.Repeat("x", 100)
	r := New(10, NewCWD())

	got := r.Resolve(tabWithCWD("/Users/dev/" + long))
	if len([]rune(got.Name)) != 10 {
		t.Errorf("name %q has %d runes, want 10", got.Name, len([]rune(got.Name)))
	}
}

func TestResolveIsDeterministic(t *testing.T) {
	r := New(DefaultMaxLength, NewCWD())
	panes := []*state.PaneState{
		{ID: "wE:p1", Dir: "/Users/dev/work/dashboard"},
		{ID: "wE:p2", Dir: "/Users/dev/work/api"},
		{ID: "wE:p3", Dir: "/Users/dev/work/infra"},
	}

	// The same panes in whichever order a snapshot listed them must name the
	// tab the same way, which is what TabFrom's ordering is for.
	want := r.Resolve(state.TabFrom(herdr.TabInfo{TabID: "wE:t1"}, "", 1, panes))
	for i := range len(panes) {
		rotated := append(slices.Clone(panes[i:]), panes[:i]...)

		got := r.Resolve(state.TabFrom(herdr.TabInfo{TabID: "wE:t1"}, "", 1, rotated))
		if got != want {
			t.Fatalf("panes from %d: resolution = %+v, want %+v", i, got, want)
		}
	}
}

// higherSource stands in for a source ranking above CWD.
type higherSource struct {
	confidence int
	parts      Parts
	ok         bool
}

func (higherSource) Name() string      { return "test_source" }
func (s higherSource) Confidence() int { return s.confidence }
func (s higherSource) Resolve(*state.PaneState) (Parts, bool) {
	return s.parts, s.ok
}

func TestHigherPrioritySourceSuppliesActivity(t *testing.T) {
	r := New(DefaultMaxLength,
		higherSource{confidence: ConfidenceProcess, parts: Parts{Activity: "Tests"}, ok: true},
		NewCWD(),
	)

	got := r.Resolve(tabWithCWD("/Users/dev/work/dashboard"))
	if got.Name != "dashboard › Tests" {
		t.Errorf("name = %q, want %q", got.Name, "dashboard › Tests")
	}

	if got.Reason != "test_source" || got.Confidence != ConfidenceProcess {
		t.Errorf(
			"reason/confidence = %q/%d, want test_source/%d",
			got.Reason,
			got.Confidence,
			ConfidenceProcess,
		)
	}
}

func TestHigherPrioritySourceOverridesContext(t *testing.T) {
	r := New(
		DefaultMaxLength,
		higherSource{
			confidence: ConfidenceSSH,
			parts:      Parts{Context: "prod-01", Activity: "SSH"},
			ok:         true,
		},
		NewCWD(),
	)

	got := r.Resolve(tabWithCWD("/Users/dev/work/dashboard"))
	if got.Name != "prod-01 › SSH" {
		t.Errorf("name = %q, want %q", got.Name, "prod-01 › SSH")
	}
}

func TestSourceThatDeclinesIsSkipped(t *testing.T) {
	r := New(DefaultMaxLength,
		higherSource{ok: false},
		NewCWD(),
	)

	got := r.Resolve(tabWithCWD("/Users/dev/work/dashboard"))
	if got.Name != "dashboard" || got.Reason != "cwd" {
		t.Errorf("decision = %+v, want dashboard via cwd", got)
	}
}

func TestATabDoesNotRepeatItsWorkspace(t *testing.T) {
	// Herdr shows the workspace above its tabs, so a tab in the workspace it is
	// named after spends half its width saying what is already on screen.
	tab := tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Fix OAuth redirect",
	})
	tab.WorkspaceName = "dashboard"

	got := Default(DefaultMaxLength, DefaultBranchMaxLength, DefaultAgentFormat).Resolve(tab)
	if got.Name != "Fix OAuth redirect" {
		t.Errorf("name = %q, want %q", got.Name, "Fix OAuth redirect")
	}
}

func TestATabWithNothingElseKeepsItsContext(t *testing.T) {
	// Dropping it here would leave the tab with no name at all, which loses
	// more than it saves.
	tab := tabWithPane(&state.PaneState{Dir: "/Users/dev/work/dashboard"})
	tab.WorkspaceName = "dashboard"

	got := Default(DefaultMaxLength, DefaultBranchMaxLength, DefaultAgentFormat).Resolve(tab)
	if got.Name != "dashboard" {
		t.Errorf("name = %q, want dashboard", got.Name)
	}
}

func TestADifferentWorkspaceIsNotDropped(t *testing.T) {
	// A tab whose directory left its workspace behind is exactly the tab that
	// needs to say where it is.
	tab := tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Fix OAuth redirect",
	})
	tab.WorkspaceName = "api"

	got := Default(DefaultMaxLength, DefaultBranchMaxLength, DefaultAgentFormat).Resolve(tab)
	if want := "dashboard › Fix OAuth redirect"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestAWorkspaceWithoutAName(t *testing.T) {
	// An unnamed workspace must not make every context look like a repeat.
	tab := tabWithPane(&state.PaneState{
		Dir:           "/Users/dev/work/dashboard",
		TerminalTitle: "Fix OAuth redirect",
	})

	got := Default(DefaultMaxLength, DefaultBranchMaxLength, DefaultAgentFormat).Resolve(tab)
	if want := "dashboard › Fix OAuth redirect"; got.Name != want {
		t.Errorf("name = %q, want %q", got.Name, want)
	}
}

func TestTheShippedChainIsAWellFormedLadder(t *testing.T) {
	// Confidences used to be repeated in every result a source returned, and
	// the chain's order was a second, unchecked statement of the same ladder.
	// Now the numbers are the only statement, so they have to hold up.
	chain := Default(DefaultMaxLength, DefaultBranchMaxLength, DefaultAgentFormat)

	seen := make(map[int]string, len(chain.sources))
	previous := 0

	for i, source := range chain.sources {
		confidence := source.Confidence()

		if other, taken := seen[confidence]; taken {
			t.Errorf("%s and %s both sit at %d; the ladder has no room for ties",
				source.Name(), other, confidence)
		}

		seen[confidence] = source.Name()

		if confidence <= ConfidenceFallback {
			t.Errorf(
				"%s at %d ranks no higher than the generic fallback",
				source.Name(),
				confidence,
			)
		}

		if i > 0 && confidence >= previous {
			t.Errorf("%s at %d is not below the source before it at %d",
				source.Name(), confidence, previous)
		}

		previous = confidence
	}
}

func TestSourcesAreOrderedByConfidenceNotByArgument(t *testing.T) {
	// Listing a source out of ladder order must not change what wins.
	low := higherSource{confidence: ConfidenceCWD, parts: Parts{Activity: "low"}, ok: true}
	high := higherSource{confidence: ConfidenceAgent, parts: Parts{Activity: "high"}, ok: true}

	tab := tabWithPane(&state.PaneState{})
	for _, chain := range []*Deterministic{
		New(DefaultMaxLength, high, low),
		New(DefaultMaxLength, low, high),
	} {
		got := chain.Resolve(tab)
		if got.Name != "high" {
			t.Errorf("name = %q, want high", got.Name)
		}

		if got.Confidence != ConfidenceAgent {
			t.Errorf("confidence = %d, want %d", got.Confidence, ConfidenceAgent)
		}
	}
}

func TestTheShippedChainResolvesATabWithNoPanes(t *testing.T) {
	got := Default(DefaultMaxLength, DefaultBranchMaxLength, DefaultAgentFormat).Resolve(state.TabState{ID: "wE:t1"})
	if got.Name != GenericFallback {
		t.Errorf("name = %q, want %q", got.Name, GenericFallback)
	}
}
