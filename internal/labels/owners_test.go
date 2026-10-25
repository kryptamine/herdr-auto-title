package labels_test

import (
	"testing"

	"github.com/kryptamine/herdr-auto-title/internal/labels"
	"github.com/kryptamine/herdr-auto-title/internal/state"
)

func TestOwnersMatchMirrorsStreamerAndConfiguredNames(t *testing.T) {
	t.Parallel()

	streamer := &state.PaneState{
		Processes: []state.Process{{
			Name: "herdr-mirror",
			Args: []string{"herdr-mirror", "pane", "host", "w1:p1"},
		}},
	}
	if !labels.NewOwners(nil).Owns(streamer) {
		t.Fatal("herdr-mirror pane should be foreign")
	}

	notStreamer := &state.PaneState{
		Processes: []state.Process{{
			Name: "herdr-mirror",
			Args: []string{"herdr-mirror", "status"},
		}},
	}
	if labels.NewOwners(nil).Owns(notStreamer) {
		t.Fatal("a non-streamer herdr-mirror invocation is not foreign")
	}

	configured := &state.PaneState{
		Processes: []state.Process{{Name: "custom-agent"}},
	}
	if !labels.NewOwners([]string{"custom-agent"}).Owns(configured) {
		t.Fatal("a configured foreign process should be foreign")
	}

	ordinary := &state.PaneState{
		Processes: []state.Process{{Name: "bash"}},
	}
	if labels.NewOwners(nil).Owns(ordinary) {
		t.Fatal("an ordinary shell is not foreign")
	}
}
