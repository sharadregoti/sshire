package tui

import (
	"context"
	"fmt"
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
	waitForOutput(t, tm, "Open roles")

	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestDetailScrollsLongDescription guards the gap a long real job posting
// exposed: without a viewport, a description longer than the terminal just
// overflows unmanaged. Every filler line is numbered so the assertion is on
// the description having actually moved, rather than on a position counter
// reporting that it did.
func TestDetailScrollsLongDescription(t *testing.T) {
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = fmt.Sprintf("filler line %02d", i+1)
	}
	longDesc := strings.Join(lines, "\n")

	jobs := []model.Job{{ID: "eng-1", Title: "Backend Engineer", Location: "Remote", Type: "Full-time", Description: longDesc}}
	m := New(Options{Company: "Acme Corp", Jobs: jobs})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	waitForOutput(t, tm, "Backend Engineer")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForOutput(t, tm, "filler line 01")

	// The viewport holds 24 rows on a 40-row terminal, so the last line sits
	// far below the fold and can only appear once scrolling has happened.
	for range 10 {
		tm.Send(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	waitForOutput(t, tm, "filler line 60")

	tm.Quit()
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

// TestListShowsEveryJob guards a real bug: list keeps filtering enabled by
// default and reserves a row for the filter prompt, so the list's usable
// height was one row short of what it was given. The last job silently fell
// onto a second page nothing could navigate to (pagination is hidden), and
// the reserved row rendered as a blank line above the list.
func TestListShowsEveryJob(t *testing.T) {
	jobs := []model.Job{
		{ID: "eng-1", Title: "Backend Engineer", Location: "Remote", Type: "Full-time"},
		{ID: "eng-2", Title: "Frontend Engineer", Location: "Remote", Type: "Full-time"},
		{ID: "eng-3", Title: "Platform Engineer", Location: "Remote", Type: "Full-time"},
	}
	m := New(Options{Company: "Acme Corp", Jobs: jobs})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 35})
	m = updated.(Model)

	view := m.viewList()
	for _, j := range jobs {
		if !strings.Contains(view, j.Title) {
			t.Errorf("job %q missing from the list view:\n%s", j.Title, view)
		}
	}

	// The job rows must butt directly against "Open roles" — one blank line
	// between them, not two.
	if _, rest, ok := strings.Cut(view, "Open roles\n"); !ok {
		t.Fatalf("no %q heading in list view:\n%s", "Open roles", view)
	} else if !strings.HasPrefix(rest, "\n›") {
		t.Errorf("expected exactly one blank line between the heading and the first job, got:\n%q", rest)
	}
}

func waitForOutput(t *testing.T, tm *teatest.TestModel, substr string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
		return strings.Contains(string(bts), substr)
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(25*time.Millisecond))
}
