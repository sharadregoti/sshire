package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/sharadregoti/sshire/internal/model"
	"github.com/sharadregoti/sshire/internal/sink"
)

// captureSink records every application submitted to it, so the test can
// assert the apply flow actually reaches a sink — not just that the TUI
// renders the right screens.
type captureSink struct {
	mu   sync.Mutex
	apps []model.Application
}

func (c *captureSink) Name() string { return "capture" }
func (c *captureSink) Submit(_ context.Context, app model.Application) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apps = append(c.apps, app)
	return nil
}
func (c *captureSink) got() []model.Application {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]model.Application, len(c.apps))
	copy(out, c.apps)
	return out
}

func TestApplyFlowEndToEnd(t *testing.T) {
	capture := &captureSink{}
	jobs := []model.Job{
		{ID: "eng-1", Title: "Backend Engineer", Location: "Remote", Type: "Full-time", Description: "Build the thing."},
		{ID: "eng-2", Title: "Frontend Engineer", Location: "Remote", Type: "Full-time", Description: "Build the other thing."},
	}

	m := New(Options{
		Company:    "Acme Corp",
		Tagline:    "we build things",
		Jobs:       jobs,
		Sinks:      sink.Multi{capture},
		RemoteAddr: "203.0.113.42",
	})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))

	waitForOutput(t, tm, "Backend Engineer")

	// Select the first job.
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForOutput(t, tm, "Build the thing.")

	// Open the apply form.
	tm.Type("a")
	waitForOutput(t, tm, "Name")

	// huh advances focus to the next field via an async tea.Cmd, so typing
	// the next field's first rune immediately after Enter can race that
	// cmd's result message. A short pause (a real typist is far slower than
	// this anyway) lets the focus change land first.
	nextField := func() {
		tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(20 * time.Millisecond)
	}

	tm.Type("Ada Lovelace")
	nextField()

	tm.Type("ada@example.com")
	nextField()

	tm.Type("https://github.com/ada")
	nextField()

	tm.Type("Excited to apply!")
	nextField()

	waitForOutput(t, tm, "Application submitted")

	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	apps := capture.got()
	if len(apps) != 1 {
		t.Fatalf("expected exactly 1 submitted application, got %d", len(apps))
	}
	got := apps[0]
	if got.JobID != "eng-1" || got.Name != "Ada Lovelace" || got.Email != "ada@example.com" {
		t.Fatalf("unexpected application recorded: %+v", got)
	}
	if got.RemoteAddr != "203.0.113.42" {
		t.Fatalf("expected remote addr to be threaded through, got %q", got.RemoteAddr)
	}
}

func TestEmailApplyShowsAddressAndSkipsForm(t *testing.T) {
	jobs := []model.Job{
		{ID: "eng-1", Title: "Backend Engineer", Location: "Remote", Type: "Full-time", Description: "Build the thing.", ApplyEmail: "jobs@acme.example"},
	}
	m := New(Options{Company: "Acme Corp", Jobs: jobs, RemoteAddr: "203.0.113.42"})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 30))

	waitForOutput(t, tm, "Backend Engineer")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	// teatest.WaitFor reads destructively from the same stream, so two
	// consecutive waits for substrings that land in the same render frame
	// (as these two do) can race each other. One check on the email is
	// proof enough that the whole "To apply: <email>" line rendered.
	waitForOutput(t, tm, "jobs@acme.example")

	// "a" is the apply-form shortcut when there's no configured email; with
	// one configured it must be a no-op rather than opening a form. Prove it
	// behaviorally: if "a" had opened the form, "esc" would land back on the
	// detail screen (see updateApply), not the job list.
	tm.Type("a")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	waitForOutput(t, tm, "open roles")

	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestDetailScrollsLongDescription guards the gap a long real job posting
// exposed: without a viewport, a description longer than the terminal just
// overflows unmanaged. 40 rows minus detailChromeLines(7) is 33 visible
// lines, so a 60-line description should start at "lines 1-33 of 60" and
// advance by exactly 5 after five down-scrolls.
func TestDetailScrollsLongDescription(t *testing.T) {
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "filler line to force wrapping and scrolling"
	}
	longDesc := strings.Join(lines, "\n")

	jobs := []model.Job{{ID: "eng-1", Title: "Backend Engineer", Location: "Remote", Type: "Full-time", Description: longDesc}}
	m := New(Options{Company: "Acme Corp", Jobs: jobs})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	waitForOutput(t, tm, "Backend Engineer")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForOutput(t, tm, "lines 1-33 of 60")

	tm.Type("jjjjj")
	waitForOutput(t, tm, "lines 6-38 of 60")

	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func waitForOutput(t *testing.T, tm *teatest.TestModel, substr string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), substr)
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(25*time.Millisecond))
}
