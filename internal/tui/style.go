package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// The whole app renders as one "terminal window" card, centered on whatever
// background the viewer's own terminal already has — traffic-light dots, a
// title bar, a rule, then the screen's own content — the same look
// superlogical.jobs uses. Only the card itself is painted black; the area
// around it is left unstyled on purpose, so the surrounding terminal shows
// through instead of getting covered by a second, larger black rectangle.
// Each screen sizes the card to its own content (a short job list gets a
// small card, a long job description gets a bigger one) rather than every
// screen sharing one fixed box.
const (
	cardPadV = 1
	cardPadH = 2

	cardMinWidth = 30
	cardMarginX  = 6 // leaves breathing room so the card never touches the terminal's edges
	cardMarginY  = 4

	// proseWidth is the comfortable reading width for free-flowing text
	// (the job description, the apply form) — content gets wrapped to
	// this, clamped to the terminal, rather than measured from its
	// longest natural line the way the job list's width is. Wide enough
	// to also fit the detail screen's help+scroll-position line (up to
	// ~67 chars for a long description) without wrapping it.
	proseWidth = 74
)

// Color values are renderer-independent (they just describe an intent);
// only lipgloss.Style objects are bound to a renderer, which is why Styles
// below must be built from the session's own *lipgloss.Renderer rather than
// via the package-level lipgloss.NewStyle().
var (
	colorBG     = lipgloss.Color("#000000")
	colorFG     = lipgloss.Color("#e8e8e8")
	colorDim    = lipgloss.Color("#6b6b6b")
	colorRule   = lipgloss.Color("#2a2a2a")
	colorRed    = lipgloss.Color("#ff5f57")
	colorYellow = lipgloss.Color("#febc2e")
	colorGreen  = lipgloss.Color("#28c840")
)

// Styles are all built from one *lipgloss.Renderer, scoped to a single SSH
// session. A style built via the package-level lipgloss.NewStyle() instead
// would render color-blind: lipgloss's global default renderer detects
// color support from the *server process's own* stdout (redirected to a
// log file, not a terminal), not the connecting client's — every SSH client
// would silently get plain, uncolored text regardless of its own terminal.
type Styles struct {
	header, dim, help           lipgloss.Style
	card, titleBar, rule        lipgloss.Style
	dotRed, dotYellow, dotGreen lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) Styles {
	return Styles{
		header:    r.NewStyle().Bold(true).Foreground(colorFG),
		dim:       r.NewStyle().Foreground(colorDim),
		help:      r.NewStyle().Foreground(colorDim),
		card:      r.NewStyle().Background(colorBG).Foreground(colorFG).Padding(cardPadV, cardPadH),
		titleBar:  r.NewStyle().Foreground(colorDim),
		rule:      r.NewStyle().Foreground(colorRule),
		dotRed:    r.NewStyle().Foreground(colorRed),
		dotYellow: r.NewStyle().Foreground(colorYellow),
		dotGreen:  r.NewStyle().Foreground(colorGreen),
	}
}

// maxCardWidth is the widest a card may grow, leaving cardMarginX clear on
// each side of the terminal.
func (m Model) maxCardWidth() int {
	if m.width <= 0 {
		return proseWidth
	}
	return max(m.width-cardMarginX, cardMinWidth)
}

// maxContentHeight is how many rows a screen's own scrollable/paginated
// content (the job list, the description viewport) may grow to before it
// must start scrolling instead of just getting taller, leaving cardMarginY
// clear top and bottom. overhead is however many additional chrome/header/
// help lines that screen wraps around its content.
func (m Model) maxContentHeight(overhead int) int {
	limit := 20
	if m.height > 0 {
		limit = max(m.height-cardMarginY, 6)
	}
	return max(limit-overhead, 1)
}

// proseContentWidth is the wrap width for free-flowing text content (the
// detail description, the apply form), clamped to the terminal.
func (m Model) proseContentWidth() int {
	return min(proseWidth, m.maxCardWidth())
}

func (m Model) dots() string {
	s := m.styles
	return s.dotRed.Render("●") + " " + s.dotYellow.Render("●") + " " + s.dotGreen.Render("●")
}

func (m Model) titleBarMinWidth(title string) int {
	return lipgloss.Width(m.dots()) + 1 + lipgloss.Width(title)
}

// chrome renders the fake-terminal-window bar (dots + right-aligned title)
// and the rule beneath it, sized to innerWidth.
func (m Model) chrome(innerWidth int, title string) string {
	left := m.dots()
	right := m.styles.titleBar.Render(title)
	gap := max(innerWidth-lipgloss.Width(left)-lipgloss.Width(right), 1)
	bar := left + strings.Repeat(" ", gap) + right
	rule := m.styles.rule.Render(strings.Repeat("─", innerWidth))
	return bar + "\n" + rule
}

// naturalWidth is the widest rendered line in s.
func naturalWidth(s string) int {
	w := 0
	for line := range strings.SplitSeq(s, "\n") {
		if lw := lipgloss.Width(line); lw > w {
			w = lw
		}
	}
	return w
}

// renderCard builds a card sized to fit width (the content's own natural
// width, or a fixed prose width — see each view* function), wraps it in the
// window chrome, and centers the result on the terminal. The area outside
// the card is left unstyled so the viewer's own terminal background shows
// through around it — the card is the only thing painted black.
func (m Model) renderCard(title string, width int, body string) string {
	width = max(width, m.titleBarMinWidth(title))
	width = min(max(width, cardMinWidth), m.maxCardWidth())

	content := m.chrome(width, title) + "\n\n" + body
	box := m.styles.card.Width(width + cardPadH*2).Render(content)
	if m.width <= 0 || m.height <= 0 {
		return box
	}
	return m.renderer.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// jobDelegate is a minimal single-line list.ItemDelegate: "› Title" bold for
// the selected job, a plain dim title for the rest — no border bar, no
// description line, matching the reference's flat job list.
type jobDelegate struct{ styles Styles }

func (jobDelegate) Height() int                         { return 1 }
func (jobDelegate) Spacing() int                        { return 0 }
func (jobDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d jobDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(jobItem)
	if !ok {
		return
	}
	if index == m.Index() {
		fmt.Fprint(w, d.styles.header.Render("› "+item.job.Title))
		return
	}
	fmt.Fprint(w, d.styles.dim.Render("  "+item.job.Title))
}

// applyTheme is a minimal, monochrome huh theme matching the card's
// palette, built on huh's own grayscale ThemeBase for structure (border
// shapes, prefix strings, key bindings) with every color-bearing field
// rebuilt from the session's renderer instead of huh's own package-level
// default — the same renderer-binding concern Styles exists for.
func (m Model) applyTheme() *huh.Theme {
	r := m.renderer
	t := huh.ThemeBase()

	t.Focused.Base = r.NewStyle().PaddingLeft(1).
		BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).BorderForeground(colorRule)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = r.NewStyle().Foreground(colorFG).Bold(true)
	t.Focused.Description = r.NewStyle().Foreground(colorDim)
	t.Focused.TextInput.Text = r.NewStyle().Foreground(colorFG)
	t.Focused.TextInput.Prompt = r.NewStyle().Foreground(colorDim)
	t.Focused.TextInput.Placeholder = r.NewStyle().Foreground(colorRule)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.Title = r.NewStyle().Foreground(colorDim)

	t.Help.ShortKey = r.NewStyle().Foreground(colorDim)
	t.Help.ShortDesc = r.NewStyle().Foreground(colorRule)
	t.Help.FullKey = r.NewStyle().Foreground(colorDim)
	t.Help.FullDesc = r.NewStyle().Foreground(colorRule)
	return t
}
