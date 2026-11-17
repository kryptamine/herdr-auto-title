package files

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sync"
	"time"
)

// refresh is how long a machine's answers are reused before they are asked
// again in one batch, as a local session read is; see
// docs/architecture/desktop-api.md#connected-machines.
const refresh = 5 * time.Second

// fetchTimeout bounds one batch, which has taken over 500 ms for five panes,
// inside a poll that renames local tabs too.
const fetchTimeout = time.Second

// keep is how long the last answers stand in while a machine cannot be asked,
// so a dropped connection does not rename its tabs and back.
const keep = 30 * time.Second

// bulkLimit tells a transcript's read from a ref file's: a bulk read is not
// asked again at a refresh, and a batch carries at most bulkBudget of them,
// below the 8 MiB machine.exec returns before it truncates.
const (
	bulkLimit  = 64 << 10
	bulkBudget = 4 << 20
)

//go:embed machine.py
var helper string

// Exec runs argv on the machine with input as its stdin and returns its stdout.
type Exec func(ctx context.Context, argv []string, input []byte) ([]byte, error)

// request is one question a machine is asked. The helper answers it without
// knowing what any file means.
type request struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	From  int64  `json:"from,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type answer struct {
	Missing bool     `json:"missing"`
	Failed  bool     `json:"failed"`
	Size    int64    `json:"size"`
	MTimeNS int64    `json:"mtime_ns"`
	Dir     bool     `json:"dir"`
	Regular bool     `json:"regular"`
	Data    []byte   `json:"data"`
	Names   []string `json:"names"`
}

// Machine is a connected machine's filesystem as its last batch described it.
// A question it holds no answer to is ErrUnfetched and goes into the next
// batch, so a reader converges over a few polls and then costs one per refresh.
type Machine struct {
	mu        sync.Mutex
	answers   map[request]answer
	touched   map[request]bool
	missed    map[request]bool
	misses    int
	askedAt   time.Time
	answered  time.Time
	failed    bool
	available bool
	now       func() time.Time
}

func NewMachine() *Machine {
	return &Machine{
		answers: make(map[request]answer),
		touched: make(map[request]bool),
		missed:  make(map[request]bool),
		// Until it fails, a machine is one that will answer.
		available: true,
		now:       time.Now,
	}
}

// Fetch asks the machine, in one batch, what readers missed since the last
// and, once refresh is up, again everything they read. A failure keeps the
// last answers for keep; a machine that never answered reads as empty.
func (m *Machine) Fetch(ctx context.Context, exec Exec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	due := now.Sub(m.askedAt) >= refresh

	// After a failure the misses wait for the refresh too, so a machine that
	// cannot answer is asked once per refresh, not once per poll.
	if !due && (m.failed || len(m.missed) == 0) {
		return nil
	}

	batch := m.batch(due)
	if due {
		m.askedAt = now
	}

	if len(batch) == 0 {
		// Nothing was read since the last refresh, so nothing of it is kept.
		if due {
			m.answers = make(map[request]answer)
		}

		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	answers, err := ask(ctx, exec, batch)
	if err != nil {
		m.failed = true
		if now.Sub(m.answered) > keep {
			m.answers, m.available = make(map[request]answer), false
		}

		return err
	}

	if due {
		m.answers, m.touched = make(map[request]answer), make(map[request]bool)
	}

	for i, question := range batch {
		m.answers[question] = answers[i]
		delete(m.missed, question)
	}

	m.failed, m.available, m.answered = false, true, now

	return nil
}

// Misses counts the questions readers could not be answered, so a caller can
// tell a read that saw the whole machine from one that saw part of it.
func (m *Machine) Misses() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.misses
}

func (m *Machine) Stat(name string) (Info, error) {
	got, err := m.lookup(request{Op: "stat", Path: name})
	if err != nil {
		return Info{}, err
	}

	return Info{
		Size:    got.Size,
		ModTime: time.Unix(0, got.MTimeNS),
		Dir:     got.Dir,
		Regular: got.Regular,
	}, nil
}

func (m *Machine) ReadAt(name string, from int64, limit int) ([]byte, error) {
	got, err := m.lookup(request{Op: "read", Path: name, From: from, Limit: limit})

	return got.Data, err
}

func (m *Machine) Glob(pattern string) ([]string, error) {
	got, err := m.lookup(request{Op: "glob", Path: pattern})

	return got.Names, err
}

// Links reports where the symbolic links in dir point, as /proc/<pid>/fd lists
// what a process holds open.
func (m *Machine) Links(dir string) ([]string, error) {
	got, err := m.lookup(request{Op: "links", Path: dir})

	return got.Names, err
}

// Join and the rest take a connected machine to name its files as Linux does.
func (*Machine) Join(elem ...string) string { return path.Join(elem...) }
func (*Machine) Dir(name string) string     { return path.Dir(name) }
func (*Machine) Base(name string) string    { return path.Base(name) }
func (*Machine) IsAbs(name string) bool     { return path.IsAbs(name) }

func (m *Machine) lookup(question request) (answer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	got, known := m.answers[question]
	if !known {
		m.missed[question] = true
		// A machine that cannot be asked has nothing to wait for.
		if !m.available {
			return answer{}, fs.ErrNotExist
		}

		m.misses++

		return answer{}, ErrUnfetched
	}

	m.touched[question] = true

	switch {
	case got.Missing:
		return answer{}, fs.ErrNotExist
	case got.Failed:
		return answer{}, fs.ErrPermission
	}

	return got, nil
}

// batch is what readers missed, bulk reads within bulkBudget, and at a refresh
// every small question they asked since the last.
func (m *Machine) batch(due bool) []request {
	var batch []request

	bulk := 0

	for question := range m.missed {
		if question.Limit > bulkLimit {
			if bulk+question.Limit > bulkBudget {
				continue
			}

			bulk += question.Limit
		}

		batch = append(batch, question)
	}

	if due {
		for question := range m.touched {
			if question.Limit <= bulkLimit && !m.missed[question] {
				batch = append(batch, question)
			}
		}
	}

	return batch
}

func ask(ctx context.Context, exec Exec, batch []request) ([]answer, error) {
	input, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}

	raw, err := exec(ctx, []string{"python3", "-c", helper}, input)
	if err != nil {
		return nil, err
	}

	var answers []answer
	if err := json.Unmarshal(raw, &answers); err != nil {
		return nil, fmt.Errorf("decode the machine's answers: %w", err)
	}

	if len(answers) != len(batch) {
		return nil, errors.New("the machine answered a different batch")
	}

	// Go's Glob sorts its matches and a reader may pick by position.
	for i := range answers {
		slices.Sort(answers[i].Names)
	}

	return answers, nil
}
