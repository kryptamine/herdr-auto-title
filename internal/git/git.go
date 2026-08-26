// Package git reads what a repository has checked out from the files under
// .git, never by running git. The measurements that settled that are in
// docs/architecture/title-resolution.md.
package git

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// shortCommitLength is how much of a detached HEAD's hash names it. Seven is
// what git itself abbreviates to.
const shortCommitLength = 7

// The lengths a whole hash has: sha1, and sha256 for a repository built with
// it. Checking the length is what keeps a HEAD holding something else from
// being read as a commit.
const (
	sha1Length   = 40
	sha256Length = 64
)

// maxRefFileSize bounds what is read from a ref file. These hold one short
// line; anything larger is not one, and the paths reaching here come from a
// pane's working directory.
const maxRefFileSize = 4 << 10

// branchPrefix is what HEAD carries when a branch is checked out.
const branchPrefix = "ref: refs/heads/"

// remoteHeadPrefix is what the default-branch marker carries.
const remoteHeadPrefix = "ref: refs/remotes/origin/"

// Checkout is what the repository holding a directory has checked out.
type Checkout struct {
	// Branch is the checked-out branch, or the branch a rebase set aside.
	Branch string
	// Commit is the abbreviated hash HEAD points at, set only when no branch
	// does — a checked-out tag, a bisect, a detached HEAD.
	Commit string
	// Default is the repository's default branch, empty when it records none.
	Default string
}

// Read reports what the repository holding dir has checked out. It reports
// false for a directory outside a repository, and for one whose .git cannot be
// read: neither is an error worth a log line twice a second.
func Read(dir string) (Checkout, bool) {
	gitDir, commonDir, found := discover(dir)
	if !found {
		return Checkout{}, false
	}

	head, ok := readRef(filepath.Join(gitDir, "HEAD"))
	if !ok {
		return Checkout{}, false
	}

	checkout := Checkout{Default: defaultBranch(commonDir)}

	switch {
	case strings.HasPrefix(head, branchPrefix):
		checkout.Branch = strings.TrimPrefix(head, branchPrefix)
	default:
		// A rebase detaches HEAD and records the branch it came from, which is
		// still where the user is working.
		if branch, rebasing := rebasingBranch(gitDir); rebasing {
			checkout.Branch = branch
			break
		}

		checkout.Commit = abbreviate(head)
	}

	if checkout.Branch == "" && checkout.Commit == "" {
		return Checkout{}, false
	}

	return checkout, true
}

// discover walks up from dir to the repository holding it, returning the
// directory holding HEAD and the one holding the shared refs. The two differ in
// a worktree and in a submodule, where .git is a file pointing elsewhere.
func discover(dir string) (gitDir, commonDir string, found bool) {
	if dir == "" || !filepath.IsAbs(dir) {
		return "", "", false
	}

	for dir = filepath.Clean(dir); ; {
		candidate := filepath.Join(dir, ".git")

		info, err := os.Stat(candidate)
		switch {
		case err != nil:
		case info.IsDir():
			return candidate, candidate, true
		default:
			gitDir, ok := linkedGitDir(candidate)
			if !ok {
				return "", "", false
			}

			return gitDir, commonDirOf(gitDir), true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false
		}

		dir = parent
	}
}

// linkedGitDir follows the `gitdir:` line a worktree or submodule leaves in
// place of a .git directory. A relative path in it is relative to the directory
// the file is in.
func linkedGitDir(gitFile string) (string, bool) {
	line, ok := readRef(gitFile)
	if !ok {
		return "", false
	}

	target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if target == line || target == "" {
		return "", false
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(gitFile), target)
	}

	return filepath.Clean(target), true
}

// commonDirOf returns where a linked git directory keeps the refs it shares
// with the repository it belongs to. A worktree's own directory holds its HEAD
// but not the remote refs the default branch is read from.
func commonDirOf(gitDir string) string {
	common, ok := readRef(filepath.Join(gitDir, "commondir"))
	if !ok {
		return gitDir
	}

	if !filepath.IsAbs(common) {
		common = filepath.Join(gitDir, common)
	}

	return filepath.Clean(common)
}

// defaultBranch reads the branch the repository treats as its trunk. It is a
// symbolic ref, which `git pack-refs` leaves as a file, so there is no packed
// form to parse. A repository that was never cloned records none.
func defaultBranch(commonDir string) string {
	line, ok := readRef(filepath.Join(commonDir, "refs", "remotes", "origin", "HEAD"))
	if !ok || !strings.HasPrefix(line, remoteHeadPrefix) {
		return ""
	}

	return strings.TrimPrefix(line, remoteHeadPrefix)
}

// rebasingBranch reports the branch a rebase in progress set aside. Both
// backends record it: the merge backend is the default, the apply backend is
// what `--apply` and `git am` use.
func rebasingBranch(gitDir string) (string, bool) {
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		line, ok := readRef(filepath.Join(gitDir, state, "head-name"))
		if !ok || !strings.HasPrefix(line, "refs/heads/") {
			continue
		}

		if branch := strings.TrimPrefix(line, "refs/heads/"); branch != "" {
			return branch, true
		}
	}

	return "", false
}

// readRef returns the first line of a ref file, or false when there is nothing
// to read. Reading is bounded: these files hold one line.
func readRef(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close() //nolint:errcheck // a read-only file has nothing to report on close

	content, err := io.ReadAll(io.LimitReader(file, maxRefFileSize))
	if err != nil {
		return "", false
	}

	line, _, _ := strings.Cut(string(content), "\n")

	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}

	return line, true
}

// abbreviate shortens a detached HEAD's hash to the length git prints. A HEAD
// holding anything else yields nothing, so a file that is not a ref cannot
// become a tab label.
func abbreviate(head string) string {
	whole := len(head) == sha1Length || len(head) == sha256Length
	if !whole || !isHex(head) {
		return ""
	}

	return head[:shortCommitLength]
}

func isHex(value string) bool {
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}

	return true
}
