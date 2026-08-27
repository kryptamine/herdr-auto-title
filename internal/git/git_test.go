package git

import (
	"os"
	"path/filepath"
	"testing"
)

// repo builds a repository out of files rather than by running git, so the
// tests describe the format they parse and do not depend on what is installed.
type repo struct {
	root   string
	gitDir string
}

func newRepo(t *testing.T) repo {
	t.Helper()
	root := t.TempDir()
	r := repo{root: root, gitDir: filepath.Join(root, ".git")}
	r.write(t, filepath.Join(r.gitDir, "HEAD"), "ref: refs/heads/main\n")

	return r
}

func (r repo) write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r repo) head(t *testing.T, content string) repo {
	t.Helper()
	r.write(t, filepath.Join(r.gitDir, "HEAD"), content)

	return r
}

func (r repo) originHead(t *testing.T, branch string) repo {
	t.Helper()
	r.write(t, filepath.Join(r.gitDir, "refs", "remotes", "origin", "HEAD"),
		"ref: refs/remotes/origin/"+branch+"\n")

	return r
}

func (r repo) subdir(t *testing.T, path string) string {
	t.Helper()

	dir := filepath.Join(r.root, path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func mustRead(t *testing.T, dir string) Checkout {
	t.Helper()

	checkout := Read(dir)
	if checkout == (Checkout{}) {
		t.Fatalf("Read(%q) found no repository", dir)
	}

	return checkout
}

func TestTheCheckedOutBranchIsRead(t *testing.T) {
	r := newRepo(t).head(t, "ref: refs/heads/feature/MC-13675\n")

	if got := mustRead(t, r.root).Branch; got != "feature/MC-13675" {
		t.Errorf("branch %q, want %q", got, "feature/MC-13675")
	}
}

func TestTheRepositoryIsFoundFromASubdirectory(t *testing.T) {
	r := newRepo(t).head(t, "ref: refs/heads/side\n")

	if got := mustRead(t, r.subdir(t, "internal/app")).Branch; got != "side" {
		t.Errorf("branch %q, want %q", got, "side")
	}
}

func TestADirectoryOutsideARepositoryIsNotOne(t *testing.T) {
	if Read(t.TempDir()) != (Checkout{}) {
		t.Error("a directory with no .git reported a checkout")
	}
}

func TestOnlyAnAbsolutePathIsRead(t *testing.T) {
	// A relative path would be resolved against the plugin's own directory,
	// naming a tab after a repository nobody is looking at.
	if Read("relative/path") != (Checkout{}) {
		t.Error("a relative path reported a checkout")
	}
}

func TestTheDefaultBranchIsReadFromTheRepository(t *testing.T) {
	r := newRepo(t).head(t, "ref: refs/heads/side\n").originHead(t, "develop")

	if got := mustRead(t, r.root).Default; got != "develop" {
		t.Errorf("default %q, want %q", got, "develop")
	}
}

func TestARepositoryWithoutARemoteRecordsNoDefault(t *testing.T) {
	if got := mustRead(t, newRepo(t).root).Default; got != "" {
		t.Errorf("default %q, want none", got)
	}
}

func TestADetachedHeadIsAbbreviated(t *testing.T) {
	r := newRepo(t).head(t, "aaf1fd85f68047764760489dbfc3ecb5ab9d0cb8\n")

	checkout := mustRead(t, r.root)
	if checkout.Commit != "aaf1fd8" {
		t.Errorf("commit %q, want %q", checkout.Commit, "aaf1fd8")
	}

	if checkout.Branch != "" {
		t.Errorf("branch %q, want none", checkout.Branch)
	}
}

func TestASha256HeadIsAbbreviatedToo(t *testing.T) {
	r := newRepo(t).head(t, "9c8f2b1a0123456789abcdef0123456789abcdef0123456789abcdef08b1cd2e\n")

	if got := mustRead(t, r.root).Commit; got != "9c8f2b1" {
		t.Errorf("commit %q, want %q", got, "9c8f2b1")
	}
}

func TestAHeadHoldingSomethingElseIsNoCheckout(t *testing.T) {
	// A HEAD that is neither a ref nor a hash must not reach a tab label. A
	// hash of the wrong length is not one: that is what bounds the read.
	for _, content := range []string{"", "\n", "ref: refs/tags/v1.0.0\n", "not a hash\n",
		"aaf1fd85f68047764760489dbfc3ecb5ab9d0c\n"} {
		r := newRepo(t).head(t, content)
		if Read(r.root) != (Checkout{}) {
			t.Errorf("HEAD %q reported a checkout", content)
		}
	}
}

func TestARebaseNamesTheBranchItSetAside(t *testing.T) {
	// Both backends: the merge backend is the default, the apply backend is
	// what `--apply` and `git am` use.
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		r := newRepo(t).head(t, "e87a64205f30ad32f455ff99a335cb89669f3321\n")
		r.write(t, filepath.Join(r.gitDir, state, "head-name"), "refs/heads/topic\n")

		checkout := mustRead(t, r.root)
		if checkout.Branch != "topic" {
			t.Errorf("%s: branch %q, want %q", state, checkout.Branch, "topic")
		}

		if checkout.Commit != "" {
			t.Errorf("%s: commit %q, want none", state, checkout.Commit)
		}
	}
}

func TestAWorktreeIsFollowedToItsOwnHead(t *testing.T) {
	main := newRepo(t).originHead(t, "main")
	worktreeGitDir := filepath.Join(main.gitDir, "worktrees", "wt")
	main.write(t, filepath.Join(worktreeGitDir, "HEAD"), "ref: refs/heads/side\n")
	main.write(t, filepath.Join(worktreeGitDir, "commondir"), "../..\n")

	tree := t.TempDir()
	main.write(t, filepath.Join(tree, ".git"), "gitdir: "+worktreeGitDir+"\n")

	checkout := mustRead(t, tree)
	if checkout.Branch != "side" {
		t.Errorf("branch %q, want %q", checkout.Branch, "side")
	}
	// The default branch lives with the shared refs, not in the worktree.
	if checkout.Default != "main" {
		t.Errorf("default %q, want %q", checkout.Default, "main")
	}
}

func TestARelativeGitdirIsResolvedAgainstTheFileHoldingIt(t *testing.T) {
	// A submodule records its git directory relative to its own working tree.
	parent := newRepo(t)
	moduleGitDir := filepath.Join(parent.gitDir, "modules", "vendor")
	parent.write(t, filepath.Join(moduleGitDir, "HEAD"), "ref: refs/heads/pinned\n")

	module := parent.subdir(t, "vendor")
	parent.write(t, filepath.Join(module, ".git"), "gitdir: ../.git/modules/vendor\n")

	if got := mustRead(t, module).Branch; got != "pinned" {
		t.Errorf("branch %q, want %q", got, "pinned")
	}
}

func TestAnUnreadableGitFileIsNoRepository(t *testing.T) {
	root := t.TempDir()
	r := repo{root: root, gitDir: filepath.Join(root, ".git")}
	r.write(t, filepath.Join(root, ".git"), "not a gitdir line\n")

	if Read(root) != (Checkout{}) {
		t.Error("a .git file with no gitdir reported a checkout")
	}
}

func TestATrunkNobodyIsOnIsNoCheckout(t *testing.T) {
	// The zero Checkout is how Read says it found nothing, so a repository that
	// records a default branch but holds no readable HEAD must answer with one:
	// a Default on its own would read as a checkout.
	r := newRepo(t).originHead(t, "main").head(t, "not a hash\n")

	if got := Read(r.root); got != (Checkout{}) {
		t.Errorf("checkout = %+v, want nothing found", got)
	}
}

func TestOnlyTheFirstLineOfARefIsRead(t *testing.T) {
	// Nothing writes a second line, but a tab label is one line either way.
	r := newRepo(t).head(t, "ref: refs/heads/side\nrubbish\n")

	if got := mustRead(t, r.root).Branch; got != "side" {
		t.Errorf("branch %q, want %q", got, "side")
	}
}

func TestAHugeHeadIsNotReadWhole(t *testing.T) {
	r := newRepo(t)

	huge := make([]byte, 2*maxRefFileSize)
	for i := range huge {
		huge[i] = 'a'
	}

	r.write(t, filepath.Join(r.gitDir, "HEAD"), string(huge))

	// The bounded read leaves a truncated hash, which is not a checkout.
	if Read(r.root) != (Checkout{}) {
		t.Error("an oversized HEAD reported a checkout")
	}
}
