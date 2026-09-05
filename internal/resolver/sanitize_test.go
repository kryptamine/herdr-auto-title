package resolver

import "testing"

func TestSanitize(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{"plain value is kept", "dashboard", 64, "dashboard"},
		{"ansi colour codes are stripped", "\x1b[31mdashboard\x1b[0m", 64, "dashboard"},
		{"osc title sequence is stripped", "\x1b]0;dashboard\x07api", 64, "api"},
		{"newlines become spaces", "fix oauth\nredirect", 64, "fix oauth redirect"},
		{"carriage returns and tabs collapse", "a\r\n\tb", 64, "a b"},
		{"repeated whitespace collapses", "dashboard    tests", 64, "dashboard tests"},
		{"repeated separators collapse", "dashboard ›  › tests", 64, "dashboard › tests"},
		{"separator spacing is normalized", "dashboard›tests", 64, "dashboard › tests"},
		{"leading and trailing separators are trimmed", "› dashboard ›", 64, "dashboard"},
		{"surrounding whitespace is trimmed", "   dashboard   ", 64, "dashboard"},
		{"control characters are removed", "dash\x00board\x07", 64, "dashboard"},
		{
			"a non-breaking space collapses like any other",
			"dash\u00a0\u00a0board",
			64,
			"dash board",
		},
		{"an ideographic space collapses too", "dash\u3000board", 64, "dash board"},
		{"empty input yields empty output", "", 64, ""},
		{"whitespace only yields empty output", "   \n\t ", 64, ""},
		{"escape only yields empty output", "\x1b[0m", 64, ""},
		{"value at the limit is kept whole", "abcdefghij", 10, "abcdefghij"},
		{"value over the limit is truncated", "abcdefghijk", 10, "abcdefghij"},
		{"truncation leaves no dangling separator", "abcdefg › hij", 9, "abcdefg"},
		// Non-ASCII on purpose: cutting these two-byte characters by byte
		// count would corrupt the result. They are one column each, so the
		// limit still reaches eight of them.
		{"truncation counts columns not bytes", "проектная-работа", 8, "проектна"},
		// A limit on a title is a limit on the room it takes in the tab bar.
		{"a double-width character costs two columns", "設定ファイル", 6, "設定フ"},
		{"an odd limit leaves a wide character out", "設定ファイル", 5, "設定"},
		{"nothing is cut when the whole value fits", "設定ファイル", 64, "設定ファイル"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sanitize(tc.in, tc.maxLen); got != tc.want {
				t.Errorf("Sanitize(%q, %d) = %q, want %q", tc.in, tc.maxLen, got, tc.want)
			}
		})
	}
}

func TestSanitizeRemovesFormatCharacters(t *testing.T) {
	// Invisible by definition, so each case names what it would forge.
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"a right-to-left override cannot reverse the label", "safe\u202egnp.txt", "safegnp.txt"},
		{"a zero-width space cannot forge a second dashboard", "dash\u200bboard", "dashboard"},
		{
			"a bidi isolate cannot reorder what surrounds it",
			"\u2066prod\u2069 deploy",
			"prod deploy",
		},
		{"a soft hyphen cannot split a word invisibly", "dash\u00adboard", "dashboard"},
		{"a word joiner is not a word", "dash\u2060board", "dashboard"},
		{"a byte order mark is not part of the title", "\ufeffdashboard", "dashboard"},
		{"format characters alone leave nothing", "\u200b\u202e", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sanitize(tc.in, 64); got != tc.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeKeepsTheZeroWidthJoiner(t *testing.T) {
	// The one format character a title may carry: without it the family emoji
	// truncation is careful to keep whole falls apart into four people.
	family := "work \U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466"
	if got := Sanitize(family, 64); got != family {
		t.Errorf("Sanitize(%q) = %q, want it unchanged", family, got)
	}
}

func TestSanitizeIsIdempotent(t *testing.T) {
	in := "\x1b[31mdashboard\x1b[0m ›  › tests\n"

	once := Sanitize(in, 64)
	if twice := Sanitize(once, 64); twice != once {
		t.Errorf("Sanitize is not idempotent: %q then %q", once, twice)
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name  string
		parts Parts
		want  string
	}{
		{
			"context and activity are joined",
			Parts{Context: "dashboard", Activity: "Tests"},
			"dashboard › Tests",
		},
		{
			// The agent's name sits between where the user is and what the
			// agent is doing, and holds that place with either neighbour gone.
			"every part is joined in reading order",
			Parts{Context: "dashboard", Branch: "feat/oauth", Agent: "claude", Activity: "Scopes"},
			"dashboard › feat/oauth › claude › Scopes",
		},
		{
			"an agent that reported nothing stands alone",
			Parts{Context: "dashboard", Agent: "claude"},
			"dashboard › claude",
		},
		{"context alone carries no separator", Parts{Context: "dashboard"}, "dashboard"},
		{"activity alone carries no separator", Parts{Activity: "Tests"}, "Tests"},
		{"nothing yields nothing", Parts{}, ""},
		{
			"untrusted parts are sanitized",
			Parts{Context: "\x1b[31mdash\nboard", Activity: "Tests "},
			"dash board › Tests",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Format(tc.parts, 64); got != tc.want {
				t.Errorf("Format(%+v) = %q, want %q", tc.parts, got, tc.want)
			}
		})
	}
}

func TestTruncationNeverCutsAGraphemeClusterOpen(t *testing.T) {
	// Several code points can make the one character a reader sees: a family
	// emoji is four joined by zero-width joiners. Cutting inside one leaves
	// half a character, ending on an invisible joiner.
	tests := []struct {
		name     string
		in       string
		maxWidth int
		want     string
	}{
		{
			"a cluster that fits is kept whole",
			"work \U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466",
			7,
			"work \U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466",
		},
		{
			"a cluster that does not fit is dropped whole",
			"work \U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466",
			6,
			"work",
		},
		{"a skin tone stays with its emoji", "review \U0001F44D\U0001F3FD ok", 8, "review"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Sanitize(tc.in, tc.maxWidth); got != tc.want {
				t.Errorf("Sanitize(%q, %d) = %q, want %q", tc.in, tc.maxWidth, got, tc.want)
			}
		})
	}
}

func TestFormatTruncatesAssembledTitle(t *testing.T) {
	got := Format(Parts{Context: "dashboard", Activity: "OAuth scopes"}, 12)
	if got != "dashboard" {
		t.Errorf("Format truncated to %q, want %q", got, "dashboard")
	}
}
