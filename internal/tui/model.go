// Package tui is the bubbletea application a candidate sees once they SSH
// in: a job list, a job detail screen, a huh-powered apply form, and a
// confirmation screen. One Model instance is created per SSH session.
package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"github.com/sharadregoti/sshire/internal/model"
	"github.com/sharadregoti/sshire/internal/sink"
)

type state int

const (
	stateList state = iota
	stateDetail
	stateApply
	stateDone
)

type jobItem struct{ job model.Job }

func (i jobItem) Title() string       { return i.job.Title }
func (i jobItem) Description() string { return fmt.Sprintf("%s · %s", i.job.Location, i.job.Type) }
func (i jobItem) FilterValue() string { return i.job.Title }

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// Options configures a new Model. Company/Tagline/DefaultApplyEmail come
// from config.yaml's top-level company block; Jobs/Sinks/RemoteAddr are
// per-server and per-session respectively.
type Options struct {
	Company           string
	Tagline           string
	DefaultApplyEmail string
	Jobs              []model.Job
	Sinks             sink.Multi
	RemoteAddr        string
}

type Model struct {
	company           string
	tagline           string
	defaultApplyEmail string
	jobs              []model.Job
	sinks             sink.Multi
	remoteAddr        string

	state  state
	list   list.Model
	job    model.Job
	detail viewport.Model
	form   *huh.Form

	// Pointers, not plain strings: bubbletea copies Model by value on every
	// Update call, so a huh field bound to &m.someStringField would write
	// into whichever copy existed at bind time and vanish on the next
	// keystroke. Binding to a stable *string (allocated once, carried by
	// value as a pointer) keeps every copy of Model pointing at the same
	// backing memory.
	applyName, applyEmail, applyResume, applyMessage *string

	width, height int
	err           error
}

func New(opts Options) Model {
	items := make([]list.Item, len(opts.Jobs))
	for i, j := range opts.Jobs {
		items[i] = jobItem{job: j}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = fmt.Sprintf("%s — open roles", opts.Company)
	l.SetShowStatusBar(false)

	return Model{
		company:           opts.Company,
		tagline:           opts.Tagline,
		defaultApplyEmail: opts.DefaultApplyEmail,
		jobs:              opts.Jobs,
		sinks:             opts.Sinks,
		remoteAddr:        opts.RemoteAddr,
		state:             stateList,
		list:              l,
		detail:            viewport.New(0, 0),
	}
}

// detailChromeLines is everything viewDetail wraps around the scrollable
// description: the outer Padding(1, 2)'s top+bottom lines, title, meta,
// a blank line, another blank line, and the help/scroll-position line.
const detailChromeLines = 7

func (m *Model) sizeDetail() {
	m.detail.Width = m.width
	m.detail.Height = max(m.height-detailChromeLines, 1)
}

func (m *Model) openJob(job model.Job) {
	m.job = job
	content := job.Description
	if email := m.effectiveApplyEmail(); email != "" {
		content += "\n\nTo apply: " + email
	}
	m.sizeDetail()
	m.detail.SetContent(content)
	m.detail.GotoTop()
}

// effectiveApplyEmail returns the "just email us" address for the currently
// viewed job, falling back to the company-wide default. Empty means this
// job uses the in-TUI apply form instead.
func (m Model) effectiveApplyEmail() string {
	if m.job.ApplyEmail != "" {
		return m.job.ApplyEmail
	}
	return m.defaultApplyEmail
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		m.sizeDetail()
		if m.form != nil {
			m.form = m.form.WithWidth(msg.Width)
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.state {
		case stateList:
			return m.updateList(msg)
		case stateDetail:
			return m.updateDetail(msg)
		case stateApply:
			return m.updateApply(msg)
		case stateDone:
			return m, tea.Quit
		}
	}

	if m.state == stateApply && m.form != nil {
		return m.updateApply(msg)
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		if item, ok := m.list.SelectedItem().(jobItem); ok {
			m.openJob(item.job)
			m.state = stateDetail
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateList
		return m, nil
	case "q":
		return m, tea.Quit
	case "a":
		if m.effectiveApplyEmail() != "" {
			return m, nil // apply instructions are already on screen; no form to open
		}
		m.applyName, m.applyEmail, m.applyResume, m.applyMessage = new(string), new(string), new(string), new(string)
		m.form = newApplyForm(m.applyName, m.applyEmail, m.applyResume, m.applyMessage)
		m.form = m.form.WithWidth(m.width)
		m.state = stateApply
		return m, m.form.Init()
	}
	// Anything else (arrows, j/k, pgup/pgdn, u/d, g/G, ...) scrolls the
	// description, courtesy of viewport's own pager-style keymap.
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m Model) updateApply(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "esc" && m.form.State != huh.StateCompleted {
		m.state = stateDetail
		m.form = nil
		return m, nil
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		app := model.Application{
			JobID:       m.job.ID,
			JobTitle:    m.job.Title,
			Name:        *m.applyName,
			Email:       *m.applyEmail,
			ResumeLink:  *m.applyResume,
			Message:     *m.applyMessage,
			SubmittedAt: time.Now(),
			RemoteAddr:  m.remoteAddr,
		}
		go m.sinks.Submit(context.Background(), app)
		m.state = stateDone
		return m, nil
	}
	return m, cmd
}

func newApplyForm(name, email, resume, message *string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Name").Value(name).Validate(required("name")),
			huh.NewInput().Title("Email").Value(email).Validate(required("email")),
			huh.NewInput().Title("Resume link (or GitHub/LinkedIn)").Value(resume),
			huh.NewText().Title("Anything else?").Value(message),
		),
	)
}

func required(field string) func(string) error {
	return func(s string) error {
		if s == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

func (m Model) View() string {
	switch m.state {
	case stateList:
		return m.list.View()
	case stateDetail:
		return m.viewDetail()
	case stateApply:
		if m.form == nil {
			return ""
		}
		return m.form.View()
	case stateDone:
		return m.viewDone()
	}
	return ""
}

func (m Model) viewDetail() string {
	help := "↑/↓ scroll · esc roles · q quit"
	if m.effectiveApplyEmail() == "" {
		help = "a apply · " + help
	}

	total := m.detail.TotalLineCount()
	last := min(m.detail.YOffset+m.detail.VisibleLineCount(), total)
	position := fmt.Sprintf("lines %d-%d of %d", m.detail.YOffset+1, last, total)

	body := fmt.Sprintf(
		"%s\n%s\n\n%s\n\n%s",
		headerStyle.Render(m.job.Title),
		dimStyle.Render(fmt.Sprintf("%s · %s", m.job.Location, m.job.Type)),
		m.detail.View(),
		helpStyle.Render(help+"    "+position),
	)
	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}

func (m Model) viewDone() string {
	body := fmt.Sprintf(
		"%s\n\n%s applied to %s. We'll be in touch at %s.\n\n%s",
		headerStyle.Render("Application submitted ✓"),
		*m.applyName, m.job.Title, *m.applyEmail,
		helpStyle.Render("press any key to exit"),
	)
	return lipgloss.NewStyle().Padding(1, 2).Render(body)
}
