package antigravity

import (
	"os"
	"path/filepath"
	"testing"
)

const session = "a98ff10a-2d0a-4e86-9ee9-ca70a807619d"

// home is an Antigravity CLI state directory built out of files, so the tests
// describe the annotation format they parse rather than depend on the agent.
type home struct {
	t    *testing.T
	root string
}

func newHome(t *testing.T) home {
	t.Helper()

	return home{t: t, root: t.TempDir()}
}

// annotate lays down a session's annotation, replacing whatever was there.
func (h home) annotate(content string) {
	h.t.Helper()

	path := filepath.Join(h.root, "annotations", session+".pbtxt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

func TestAnAnnotationNamesASession(t *testing.T) {
	t.Parallel()

	h := newHome(t)
	h.annotate(`title:"Casual Greeting And Request"` + "\n")

	if got := NewReader(h.root).Title(session); got != "Casual Greeting And Request" {
		t.Errorf("title = %q, want the annotated title", got)
	}
}

func TestASessionIDThatIsNotOneReadsNothing(t *testing.T) {
	t.Parallel()

	// The id arrives over the socket and becomes part of a path, so one that
	// walks out of the annotations directory must not reach the file it names.
	h := newHome(t)
	h.annotate(`title:"Leaked"`)

	escape := filepath.Join("..", "annotations", session)
	if got := NewReader(h.root).Title(escape); got != "" {
		t.Errorf("title = %q, want nothing for an id that is a path", got)
	}
}

func TestAnEscapedTitleIsReadAsTheTextItSpells(t *testing.T) {
	t.Parallel()

	// Text format may spell a non-ASCII title as octal escapes of its UTF-8
	// bytes, or write those bytes as they are; both name the same session.
	for _, annotation := range []string{
		`title:"Revisi\303\263n de \"Tareas\""`,
		`title:"Revisión de \"Tareas\""`,
	} {
		h := newHome(t)
		h.annotate(annotation)

		if got := NewReader(h.root).Title(session); got != `Revisión de "Tareas"` {
			t.Errorf("title of %s = %q, want the unescaped text", annotation, got)
		}
	}
}

func TestASessionWithNoAnnotationReadsNothing(t *testing.T) {
	t.Parallel()

	if got := NewReader(newHome(t).root).Title(session); got != "" {
		t.Errorf("title = %q, want nothing before the agent annotates", got)
	}
}
