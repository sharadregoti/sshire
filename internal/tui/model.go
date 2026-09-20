// Package tui is the bubbletea application a candidate sees once they SSH
// in: a job list, a job detail screen, a huh-powered apply form, and a
// confirmation screen. One Model instance is created per SSH session.
package tui

import (
	"context"
	"fmt"
	"strings"
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

// Options configures a new Model. Company/Tagline/DefaultApplyEmail come
// from config.yaml's top-level company block; Jobs/Sinks/RemoteAddr are
// per-server and per-session respectively. Renderer should be the session's
// own *lipgloss.Renderer (see bubbletea.MakeRenderer) so styling reflects
// that client's actual terminal capabilities; nil falls back to lipgloss's
// package-level default renderer, which is fine for tests but wrong for a
// real SSH session (see Styles' doc comment in style.go).
type Options struct {
	Company           string
	Tagline           string
	DefaultApplyEmail string
	Jobs              []model.Job
	Sinks             sink.Multi
	RemoteAddr        string
	Renderer          *lipgloss.Renderer

	// Theme is the palette to render in. The zero value is not a usable
	// theme; leave it unset and New substitutes the default.
	Theme Theme
}

type Model struct {
	company           string
	tagline           string
	defaultApplyEmail string
	jobs              []model.Job
	sinks             sink.Multi
	remoteAddr        string
	renderer          *lipgloss.Renderer
	theme             Theme
	styles            Styles

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
	renderer := opts.Renderer
	if renderer == nil {
		renderer = lipgloss.DefaultRenderer()
	}
	theme := opts.Theme
	if theme.Name == "" {
		theme = themes[DefaultThemeName]
	}
	styles := newStyles(renderer, theme)

	items := make([]list.Item, len(opts.Jobs))
	for i, j := range opts.Jobs {
		items[i] = jobItem{job: j}
	}
	l := list.New(items, jobDelegate{styles: styles}, 0, 0)
	// The card's own chrome (dots + title bar) replaces list's built-in
	// title/status/pagination/help chrome entirely; "Open roles" and the
	// key hints are rendered by viewList instead.
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	// Not just cosmetic: filtering is on by default, and list reserves a row
	// for the filter prompt whenever it's enabled — which both pushed the
	// last job onto an invisible second page and rendered that reserved row
	// as a blank line above the list.
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	return Model{
		company:           opts.Company,
		tagline:           opts.Tagline,
		defaultApplyEmail: opts.DefaultApplyEmail,
		jobs:              opts.Jobs,
		sinks:             opts.Sinks,
		remoteAddr:        opts.RemoteAddr,
		renderer:          renderer,
		theme:             theme,
		styles:            styles,
		state:             stateList,
		list:              l,
		detail:            viewport.New(0, 0),
	}
}

// detailOverhead is every line renderCard and viewDetail wrap around the
// scrollable description: the card's own border (2) and top+bottom padding
// (2), the chrome bar+rule (2) and the blank line under it (1), the job
// title (1), the meta line (1), a blank line (1), and the trailing
// blank+help line (2) — 12 total.
const detailOverhead = 12

// resizeDetail re-wraps the current job's description to the terminal's
// current width and re-derives the viewport's height from how many lines
// that actually took, capped so a long description scrolls instead of
// pushing the card past the screen. Called both when a job is opened and
// on every resize; scroll position resets on resize since a re-wrap at a
// different width changes what "line 12" even means.
func (m *Model) resizeDetail() {
	width := max(m.proseContentWidth()-scrollGutter, 1)
	content := m.job.Description
	if email := m.effectiveApplyEmail(); email != "" {
		content += "\n\nTo apply: " + email
	}
	wrapped := m.styles.dim.Foreground(m.theme.FG).Width(width).Render(content)
	neededLines := strings.Count(wrapped, "\n") + 1

	m.detail.Width = width
	m.detail.Height = min(neededLines, m.maxContentHeight(detailOverhead))
	m.detail.SetContent(wrapped)
	m.detail.GotoTop()
}

func (m *Model) openJob(job model.Job) {
	m.job = job
	m.resizeDetail()
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
		m.list.SetSize(m.listContentWidth(), min(len(m.jobs), m.maxContentHeight(listOverhead)))
		m.resizeDetail()
		if m.form != nil {
			m.form = m.form.WithWidth(m.proseContentWidth())
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
		// No WithHeight: the form should hug its own fields, not stretch to
		// fill some fixed budget and leave empty space beneath them.
		m.form = m.form.WithWidth(m.proseContentWidth()).WithTheme(m.applyTheme())
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

func (m Model) title() string {
	return fmt.Sprintf("%s — Jobs", m.company)
}

func (m Model) View() string {
	switch m.state {
	case stateList:
		return m.renderCard(m.title(), m.listContentWidth(), m.viewList())
	case stateDetail:
		return m.renderCard(m.title(), m.proseContentWidth(), m.viewDetail())
	case stateApply:
		if m.form == nil {
			return ""
		}
		return m.renderCard(m.title(), m.proseContentWidth(), m.form.View())
	case stateDone:
		return m.renderCard(m.title(), m.proseContentWidth(), m.viewDone())
	}
	return ""
}

// listOverhead is every line viewList and renderCard wrap around the job
// list itself: card border (2) and padding (2), chrome+rule+blank (3),
// "Open roles" (1), a blank line, and the trailing blank+help line (2) —
// 11 total.
const listOverhead = 11

// listHelpText is the list screen's help line; listContentWidth needs its
// width too so the line doesn't wrap, and viewList renders the same text.
const listHelpText = "↑/↓ move  ·  enter open  ·  q/esc quit"

// listContentWidth sizes the card to the longest job title actually being
// shown — a two-job list gets a narrow card; a job with a long title
// widens it — rather than every list sharing one fixed width. Must also
// fit "Open roles" and the help line, or one of those wraps instead.
func (m Model) listContentWidth() int {
	w := max(lipgloss.Width("Open roles"), lipgloss.Width(listHelpText))
	for _, j := range m.jobs {
		if lw := lipgloss.Width("› " + j.Title); lw > w {
			w = lw
		}
	}
	return w
}

func (m Model) viewList() string {
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		m.styles.header.Render("Open roles"),
		m.list.View(),
		m.styles.help.Render(listHelpText),
	)
}

func (m Model) viewDetail() string {
	help := "↑/↓ scroll  ·  esc roles  ·  q quit"
	if m.effectiveApplyEmail() == "" {
		help = "a apply  ·  " + help
	}

	body := m.detail.View()
	if bar := m.scrollbar(m.detail.Height); bar != "" {
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, bar)
	}

	return fmt.Sprintf(
		"%s\n%s\n\n%s\n\n%s",
		m.styles.header.Render(m.job.Title),
		m.styles.dim.Render(fmt.Sprintf("%s · %s", m.job.Location, m.job.Type)),
		body,
		m.styles.help.Render(help),
	)
}

func (m Model) viewDone() string {
	// Wrapped to the shared card width rather than left to set its own: a
	// long name or job title would otherwise stretch this one screen wider
	// than every other, and the window would jump on the final keystroke.
	summary := fmt.Sprintf("%s applied to %s. We'll be in touch at %s.",
		*m.applyName, m.job.Title, *m.applyEmail)

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		m.styles.header.Render("Application submitted ✓"),
		m.styles.dim.Foreground(m.theme.FG).Width(m.proseContentWidth()).Render(summary),
		m.styles.help.Render("press any key to exit"),
	)
}
