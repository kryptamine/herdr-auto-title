package resolver

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/git"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

// repoPane builds a pane sitting in a repository with branch checked out and
// origin/HEAD naming defaultBranch.
func repoPane(branch, defaultBranch string) *state.PaneState {
	return &state.PaneState{
		Dir: dashboard,
		Git: git.Checkout{Branch: branch, Default: defaultBranch},
	}
}

func resolveRepoPane(pane *state.PaneState) string {
	return defaultChain().Resolve(tabWithPane(pane)).Name
}

// agentWorktree gives the pane's agent a checkout of the pane's own
// repository, as a worktree of it is: the two share the directory the refs
// live in.
func agentWorktree(pane *state.PaneState, branch string) *state.PaneState {
	common := filepath.Join(dashboard, ".git")

	pane.Git.CommonDir = common
	pane.AgentGit = git.Checkout{
		Branch:    branch,
		Default:   pane.Git.Default,
		CommonDir: common,
	}

	return pane
}

func TestTheAgentsBranchIsTheOneTheTabShows(t *testing.T) {
	t.Parallel()

	pane := agentWorktree(repoPane("main", "main"), "feat/oauth")

	if got := resolveRepoPane(pane); got != "dashboard › feat/oauth" {
		t.Errorf("title %q, want the branch the agent is working on", got)
	}
}

func TestAnAgentOnTheTrunkLeavesThePanesBranchStanding(t *testing.T) {
	t.Parallel()

	// The trunk is what a tab named after its repository already says, so the
	// pane's own branch is the only segment either checkout is worth.
	pane := agentWorktree(repoPane("feat/oauth", "main"), "main")

	if got := resolveRepoPane(pane); got != "dashboard › feat/oauth" {
		t.Errorf("title %q, want the pane's own branch", got)
	}
}

func TestAnAgentWithNoCheckoutLeavesThePanesBranchStanding(t *testing.T) {
	t.Parallel()

	// A pane whose agent is in no repository has nothing to compare against an
	// empty common directory, so the empty label is what keeps its branch.
	pane := repoPane("feat/oauth", "main")

	if got := resolveRepoPane(pane); got != "dashboard › feat/oauth" {
		t.Errorf("title %q, want the pane's own branch", got)
	}
}

func TestAnAgentInAnotherRepositoryLeavesThePanesBranchStanding(t *testing.T) {
	t.Parallel()

	// A branch from another project beside this one's name labels the wrong
	// work, which is worse than the tab carrying no branch at all.
	pane := repoPane("feat/oauth", "main")
	pane.Git.CommonDir = filepath.Join(dashboard, ".git")
	pane.AgentGit = git.Checkout{
		Branch:    "fix/token",
		Default:   "main",
		CommonDir: filepath.Join(api, ".git"),
	}

	if got := resolveRepoPane(pane); got != "dashboard › feat/oauth" {
		t.Errorf("title %q, want the pane's own branch", got)
	}
}

func TestTheBranchQualifiesTheDirectory(t *testing.T) {
	t.Parallel()

	pane := repoPane("feat/oauth", "main")
	pane.TerminalTitle = "auth.ts - Nvim"
	pane.Processes = []state.Process{{Name: "nvim"}}

	if got := resolveRepoPane(pane); got != "dashboard › feat/oauth › nvim › auth.ts" {
		t.Errorf("title %q, want the branch between the directory and the work", got)
	}
}

func TestTheDefaultBranchSaysNothing(t *testing.T) {
	t.Parallel()

	// A tab in a repository it is already named after learns nothing from
	// being told it is on that repository's trunk.
	if got := resolveRepoPane(repoPane("main", "main")); got != "dashboard" {
		t.Errorf("title %q, want just the directory", got)
	}
}

func TestTheDefaultBranchIsTheRepositorysOwn(t *testing.T) {
	t.Parallel()

	// A team whose trunk is `develop` gets the same silence, and one that works
	// on a branch named `main` off a `develop` trunk still sees it.
	if got := resolveRepoPane(repoPane("develop", "develop")); got != "dashboard" {
		t.Errorf("develop as trunk → %q, want just the directory", got)
	}

	if got := resolveRepoPane(repoPane("main", "develop")); got != "dashboard › main" {
		t.Errorf("main off a develop trunk → %q, want the branch", got)
	}
}

func TestTheTrunkIsMatchedAsGitStoresIt(t *testing.T) {
	t.Parallel()

	// Git refs are case-sensitive, so `Main` beside a `main` trunk is another
	// branch and has something to say.
	if got := resolveRepoPane(repoPane("Main", "main")); got != "dashboard › Main" {
		t.Errorf("title %q, want the branch", got)
	}
}

func TestARepositoryRecordingNoDefaultStillHasATrunk(t *testing.T) {
	t.Parallel()

	// A repository with no remote records no default, and every tab in it read
	// `main` as though it were a branch worth the width.
	for _, branch := range []string{"main", "master", "trunk"} {
		if got := resolveRepoPane(repoPane(branch, "")); got != "dashboard" {
			t.Errorf("%s with no default → %q, want just the directory", branch, got)
		}
	}
}

func TestADetachedHeadShowsTheCommit(t *testing.T) {
	t.Parallel()

	pane := &state.PaneState{
		Dir: dashboard,
		Git: git.Checkout{Commit: "a1b2c3d", Default: "main"},
	}

	if got := resolveRepoPane(pane); got != "dashboard › a1b2c3d" {
		t.Errorf("title %q, want the commit", got)
	}
}

func TestABranchIsReducedToWhatIdentifiesIt(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		// An issue key identifies the work whatever wraps it.
		"bugfix-asatretdinov-cpanel-uapi-mc-13675": "MC-13675",
		"feature/MC-13675":                         "MC-13675",
		"MC-13675":                                 "MC-13675",

		// Without one, the namespace goes and the rest is cut at a whole word.
		"docs/install-needs-a-restart": "install",
		"test/process-read-flake":      "process-read",
		"feature/add-dark-mode":        "add-dark",

		// A name that fits is left whole, key or no key: `feat/` is what tells
		// it from `fix/`, and a key that already fits needs no rescuing.
		"feat/oauth": "feat/oauth",
		"feat/ab-12": "feat/ab-12",
		"short":      "short",
	}

	for branch, want := range cases {
		if got := shortenBranch(branch, DefaultBranchMaxLength); got != want {
			t.Errorf("%s → %q, want %q", branch, got, want)
		}
	}
}

func TestABranchIsNotShownForARemoteMachine(t *testing.T) {
	t.Parallel()

	// The branch is read from the directory ssh was launched in, which says
	// nothing about the machine on the other end.
	pane := repoPane("feat/oauth", "main")
	pane.Processes = []state.Process{
		{Name: "fish", Args: []string{"-fish"}},
		{Name: "ssh", Args: strings.Fields("ssh root@prod-01")},
	}

	if got := resolveRepoPane(pane); got != "ssh › prod-01" {
		t.Errorf("title %q, want no branch beside the host", got)
	}
}

func TestAWorktreeNamedAfterItsBranchSaysItOnce(t *testing.T) {
	t.Parallel()

	// `git worktree add ../dashboard dashboard` makes a directory and a branch
	// of the same name, and the two are one fact rather than two.
	pane := repoPane("dashboard", "main")
	pane.TerminalTitle = "auth.ts - Nvim"
	pane.Processes = []state.Process{{Name: "nvim"}}

	if got := resolveRepoPane(pane); got != "dashboard › nvim › auth.ts" {
		t.Errorf("title %q, want the branch said once", got)
	}
}

func TestTheBranchSurvivesTheWorkspaceItRepeats(t *testing.T) {
	t.Parallel()

	// Herdr shows the workspace above the tabs, so the directory goes — but
	// the branch is exactly what tells two tabs of that workspace apart.
	tab := tabWithPane(repoPane("feat/oauth", "main"))
	tab.WorkspaceName = "dashboard"

	got := defaultChain().Resolve(tab).Name
	if got != "feat/oauth" {
		t.Errorf("title %q, want the branch alone", got)
	}
}

func TestABranchWidthOfZeroLeavesBranchesOut(t *testing.T) {
	t.Parallel()

	got := Default(
		Options{MaxLength: DefaultMaxLength},
	).Resolve(tabWithPane(repoPane("feat/oauth", "main"))).
		Name
	if got != "dashboard" {
		t.Errorf("title %q, want no branch at all", got)
	}
}

func TestAPromptCarryingTheBranchDoesNotSayItTwice(t *testing.T) {
	t.Parallel()

	pane := repoPane("feat/oauth", "main")
	pane.TerminalTitle = "feat/oauth"

	if got := resolveRepoPane(pane); got != "dashboard › feat/oauth" {
		t.Errorf("title %q, want the branch once", got)
	}
}

func TestABranchStandsWhereTheDirectorySaysNothing(t *testing.T) {
	t.Parallel()

	// A home directory contributes no context, but a repository checked out in
	// it — dotfiles — still says where the user is.
	pane := &state.PaneState{Git: git.Checkout{Branch: "feat/oauth", Default: "main"}}

	if got := resolveRepoPane(pane); got != "feat/oauth" {
		t.Errorf("title %q, want the branch", got)
	}
}

func TestTheBranchIsCreditedLikeAContext(t *testing.T) {
	t.Parallel()

	// The reason a tab carries a name is the source that named the work, and a
	// branch is not work: it answers for the title only when nothing else does.
	resolver := defaultChain()

	pane := repoPane("feat/oauth", "main")

	pane.TerminalTitle = "auth.ts - Nvim"
	if got := resolver.Resolve(tabWithPane(pane)).Reason; got != "terminal_title" {
		t.Errorf("reason %q, want the source of the activity", got)
	}

	if got := resolver.Resolve(tabWithPane(repoPane("feat/oauth", "main"))).Reason; got != "git" {
		t.Errorf("reason %q, want git", got)
	}
}
