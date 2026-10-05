package resolver

import "testing"

func TestMeaningful(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{"work described in words", "Fix OAuth redirect", "Fix OAuth redirect", true},
		{"file name", "auth.ts", "auth.ts", true},
		{"host name", "prod-01", "prod-01", true},
		{
			"relative path inside a sentence",
			"Fix bug in src/auth.ts",
			"Fix bug in src/auth.ts",
			true,
		},

		{"shell name", "zsh", "", false},
		{"shell name in another case", "ZSH", "", false},
		// One list of shells, so what paneKind skips as a process is what
		// Meaningful refuses as a title. These four used to get through.
		{"a shell only the process table knew", "dash", "", false},
		{"korn shell", "ksh", "", false},
		{"c shell", "csh", "", false},
		{"the login shell", "login", "", false},
		{"the windows shells", "pwsh", "", false},
		{"the older windows shell", "PowerShell", "", false},
		{"the older windows shell with its platform", "Windows PowerShell", "", false},
		{"the oldest windows shell", "cmd", "", false},
		{"multi-word program name", "Claude Code", "", false},
		{"runtime name", "node", "", false},
		{"surrounded by whitespace", "  bash  ", "", false},
		{"empty", "", "", false},
		{"whitespace only", "   ", "", false},
		{"word that merely contains a shell name", "bashful", "bashful", true},

		{"home directory", "~", "", false},
		{"abbreviated path", "~/W/herdr-auto-title", "", false},
		{"absolute path", "/Users/dev/work/dashboard", "", false},
		{"windows path", `C:\Users\dev\work\dashboard`, "", false},
		{"windows path with forward slashes", "C:/Users/dev/work/dashboard", "", false},
		{"windows home path", `~\work\dashboard`, "", false},
		{"unc path", `\\build-01\share\dashboard`, "", false},
		{"drive letter alone is not a path", "C:", "C:", true},
		// Split on whitespace, a path with spaces would leave its tail behind
		// as if it named an activity.
		{"windows path with spaces", `C:\Program Files\PowerShell\7\pwsh.exe`, "", false},
		{"work after a windows path", `C:\work - Fix login`, "Fix login", true},
		{
			"work after a windows path with spaces",
			`C:\WINDOWS\system32\cmd.exe - python scripts/probe.py`,
			"python scripts/probe.py", true,
		},
		{"work after a home path", "~/work/app git status", "git status", true},
		{
			"editor title carrying a windows path with spaces",
			`auth.ts (C:\Program Files\app) - Nvim`,
			"auth.ts - Nvim", true,
		},
		{"a windows path ending in a spaced name", `C:\Users\Jane Doe`, "", false},
		{
			"work after a windows path ending in a spaced name",
			`C:\Users\Jane Doe - Fix login`,
			"Fix login", true,
		},
		{
			"arguments after the shell's path",
			`C:\Program Files\PowerShell\7\pwsh.exe -NoLogo`,
			"", false,
		},
		// A title that opens with a Windows path is only that path up to a
		// separator, so a command written straight after it goes with it.
		{"a title opening with a windows path", `C:\work\app git status`, "", false},
		{
			"work after a windows path inside a sentence",
			`editing C:\notes\todo.txt now`,
			"editing now", true,
		},
		{
			"editor title carrying a bracketed path ending in a spaced name",
			`auth.ts (C:\Users\Jane Doe) - Nvim`,
			"auth.ts - Nvim", true,
		},
		{
			"editor title carrying a bracketed home path with spaces",
			"auth.ts (~/My Projects/app) - Nvim",
			"auth.ts - Nvim", true,
		},
		{
			"a quoted program path in a command",
			`C:\WINDOWS\system32\cmd.exe - "C:\Program Files\nodejs\node.exe" server.js`,
			"server.js", true,
		},
		{"words between two windows paths", `Copy C:\src to D:\dst`, "Copy to", true},
		{
			"words between two windows paths with spaces",
			`Copy C:\Program Files\x to D:\dst`,
			"Copy to", true,
		},
		{
			"words after a file path inside a sentence",
			`fix C:\work\app\auth.ts where the regex \d+ fails`,
			`fix where the regex \d+ fails`, true,
		},
		// A prompt read from a transcript can open with a file it is about.
		{
			"work after a file path opening the title",
			`C:\Users\jane\Desktop\error.png what is wrong here`,
			"what is wrong here", true,
		},
		// Nothing tells a relative path straight after a directory from the
		// rest of a spaced name, so it goes with the directory.
		{
			"a relative path straight after a windows directory",
			`C:\WINDOWS\system32\cmd.exe - git diff C:\work\app src\auth.ts`,
			"git diff", true,
		},
		{
			"a bracket left open past a separator",
			"sort (/tmp/in.txt | uniq > /tmp/out.txt",
			"sort | uniq", true,
		},
		// Without an extension a directory ends nowhere in particular, so only a
		// word that the next one carries on joins it.
		{
			"words after a windows directory inside a sentence",
			`fix C:\work\app where the regex \d+ fails`,
			`fix where the regex \d+ fails`, true,
		},
		{
			"a cmd line after a windows directory",
			`C:\WINDOWS\system32\cmd.exe - cd C:\work\app & python scripts\probe.py`,
			`cd & python scripts\probe.py`, true,
		},
		{
			"a spaced windows path inside a sentence",
			`fix C:\Program Files (x86)\app\main.go where it fails`,
			"fix where it fails", true,
		},
		{"a bracket inside a windows path", `C:\Program Files (x86)\Steam\steam.exe`, "", false},
		{
			"a bracketed path holding a bracket",
			`auth.ts (C:\Program Files (x86)\app) - Nvim`,
			"auth.ts - Nvim", true,
		},
		{"a bracket ending a windows path", `C:\Users\jane\Desktop\New folder (2)`, "", false},
		{
			"an escaped backslash is not a share",
			`\\n is printed literally instead of a newline`,
			"is printed literally instead of a newline", true,
		},
		{
			"escaped backslashes inside a sentence",
			`replace \\r\\n with \n before parsing`,
			`replace with \n before parsing`, true,
		},
		{
			"a redirect is not a bracket",
			"make </dev/null && ./run >log.txt",
			"make && ./run >log.txt", true,
		},
		{"a redirect before another", "sort </tmp/in.txt >sorted.txt", "sort >sorted.txt", true},
		{
			"arguments after a quoted program path opening the title",
			`"C:\Program Files\Git\bin\bash.exe" --login -i`,
			"", false,
		},
		{"a backslash after a posix path", `vim /tmp/x.txt foo\bar`, `vim foo\bar`, true},
		{
			"a quoted posix path opening the title",
			`"~/My Projects/app" notes`,
			"notes", true,
		},
		// A closed delimiter or a clause's punctuation says where a path opening
		// the title ends, so what follows is not taken for the rest of a name.
		{
			"a bracketed windows path opening the title",
			`[C:\work\app] build failed`,
			"build failed", true,
		},
		{
			"a quoted windows folder opening the title",
			`"C:\work\app" is not found`,
			"is not found", true,
		},
		{
			"a colon after a windows path",
			`C:\work\app: command not found`,
			"command not found", true,
		},
		{"a prompt after a spaced windows path", `C:\Users\Jane Doe> npm test`, "npm test", true},

		// Herdr's own title for a Windows pane whose program has set none. It
		// names the shell and the directory, and neither is what the user is
		// doing there.
		{"herdr's title for an idle windows pane", "pwsh in dashboard", "", false},
		{"the same under cmd", "cmd in herdr-auto-title", "", false},
		{"the same in the home directory", "pwsh in ~", "", false},
		{"the windows shell by its executable", "pwsh.exe", "", false},
		{"the oldest windows shell by its executable", "cmd.exe", "", false},
		{"the older windows shell by its executable", "powershell.exe", "", false},
		{"herdr's title naming the executable", "pwsh.exe in dashboard", "", false},
		{
			"work that happens to be in something",
			"Fix login in dashboard",
			"Fix login in dashboard",
			true,
		},
		{
			"editor title carrying a windows path",
			`auth.ts (C:\Users\dev\work\dashboard) - Nvim`,
			"auth.ts - Nvim", true,
		},

		// Every one of these was observed on a live session.
		{
			"editor title carrying a home path",
			"auth.provider.ts (~/Work/self-care-portal/libs/shared-api/src) - Nvim",
			"auth.provider.ts - Nvim", true,
		},
		{
			"editor title for a file at the repository root",
			"LICENSE (~/Work/herdr-auto-title) - Nvim",
			"LICENSE - Nvim", true,
		},
		{
			"editor title carrying a uri",
			"- (oil:///Users/dev/Work/herdr-auto-title) - Nvim",
			"Nvim", true,
		},
		{"absolute path inside a sentence", "editing /etc/hosts", "editing", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := Meaningful(tc.value)
			if ok != tc.ok {
				t.Fatalf("Meaningful(%q) ok = %v, want %v", tc.value, ok, tc.ok)
			}

			if got != tc.want {
				t.Errorf("Meaningful(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestAShellPromptIsNotAnActivity(t *testing.T) {
	t.Parallel()

	// A shell titling its window after its prompt says who and where, which the
	// context already says, and never says what the user is doing. Remote
	// shells do it most, which is how it reaches a tab named after a host.
	for _, title := range []string{
		"root@psi:",
		"alex@macbook:~/work",
		"deploy@prod-01:/var/log",
		"root@psi:~",
	} {
		if got, ok := Meaningful(title); ok {
			t.Errorf("Meaningful(%q) = %q, want it rejected", title, got)
		}
	}
}

func TestValuesThatOnlyLookLikePromptsSurvive(t *testing.T) {
	t.Parallel()

	// The pattern must not swallow real work that happens to contain an @.
	for _, title := range []string{
		"Fix auth@v2: rewrite the guard",
		"deploy@prod-01",
		"npm run build:prod",
		"user@host",
	} {
		if _, ok := Meaningful(title); !ok {
			t.Errorf("Meaningful(%q) rejected a meaningful value", title)
		}
	}
}
