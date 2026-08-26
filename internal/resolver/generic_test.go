package resolver

import "testing"

func TestMeaningful(t *testing.T) {
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
		{"multi-word program name", "Claude Code", "", false},
		{"runtime name", "node", "", false},
		{"surrounded by whitespace", "  bash  ", "", false},
		{"empty", "", "", false},
		{"whitespace only", "   ", "", false},
		{"word that merely contains a shell name", "bashful", "bashful", true},

		{"home directory", "~", "", false},
		{"abbreviated path", "~/W/herdr-auto-title", "", false},
		{"absolute path", "/Users/dev/work/dashboard", "", false},

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
