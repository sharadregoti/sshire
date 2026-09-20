package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// The whole app renders as one "terminal window" card — traffic-light dots,
// a title bar, a rule, then the screen's own content — centered on a black
// canvas painted across the entire terminal. Painting the canvas rather
// than letting the viewer's own background show through is what makes the
// page look the same for every candidate: a card sitting on whatever theme
// each person happens to run reads as a stray black rectangle, not as a
// careers page. It's the alt screen, so quitting restores their terminal.
// Each screen sizes the card to its own content (a long job description
// gets a bigger one) rather than every screen sharing one fixed box.
const (
	cardPadV = 1
	cardPadH = 2

	cardMinWidth = 30
	cardMarginX  = 6 // leaves breathing room so the card never touches the terminal's edges
	cardMarginY  = 4

	// cardWidth is the content width every screen aims for, so the window
	// doesn't resize and jump sideways when a candidate opens a job. A
	// preference, not a floor: content wider than this still grows the card,
	// and a terminal too narrow for it clamps back down to cardMinWidth.
	cardWidth = 58

	// scrollGutter is the blank column plus scrollbar column reserved to the
	// right of the description. Reserved whether or not the bar is currently
	// drawn, so text wraps identically on a short and a long posting.
	scrollGutter = 2
)

// A Theme is the app's entire palette. Colour values are
// renderer-independent (they only describe an intent); only lipgloss.Style
// objects are bound to a renderer, which is why Styles below must be built
// from the session's own *lipgloss.Renderer rather than via the
// package-level lipgloss.NewStyle().
type Theme struct {
	Name string

	BG    lipgloss.Color // the canvas, painted across the whole terminal
	FG    lipgloss.Color // job titles, headings, description text
	Dim   lipgloss.Color // help lines, unselected jobs, the window title
	Rule  lipgloss.Color // card border, the rule under the title bar, scroll track
	Thumb lipgloss.Color // the scrollbar thumb

	// Dots are the three fake window controls, left to right. Deliberately
	// not named red/yellow/green: a monochrome-terminal theme renders them
	// as one hue's ramp, which is what a single-phosphor tube actually did.
	Dots [3]lipgloss.Color
}

// DefaultThemeName is what an unset or unrecognised ui.theme falls back to.
const DefaultThemeName = "black"

var themes = map[string]Theme{
	"black": {
		Name: "black", BG: "#000000", FG: "#e8e8e8", Dim: "#6b6b6b", Rule: "#3d3d3d", Thumb: "#8a8a8a",
		Dots: [3]lipgloss.Color{"#ff5f57", "#febc2e", "#28c840"},
	},
	// P1 phosphor, the VT220 look. One hue throughout, dots included.
	"phosphor": {
		Name: "phosphor", BG: "#020a04", FG: "#3bf07a", Dim: "#1c7d41", Rule: "#17532c", Thumb: "#2bbd60",
		Dots: [3]lipgloss.Color{"#114a26", "#1c7d41", "#3bf07a"},
	},
	// P3 phosphor, as shipped on IBM 3270s: warmer, easier over a long read.
	"amber": {
		Name: "amber", BG: "#0f0a02", FG: "#ffb000", Dim: "#8a5f10", Rule: "#4d3505", Thumb: "#cc8c00",
		Dots: [3]lipgloss.Color{"#4d3505", "#a3700a", "#ffb000"},
	},
	// The inverted one. Worth keeping working: it is the only theme that
	// catches code quietly assuming a dark ground.
	"paper": {
		Name: "paper", BG: "#f5f2ea", FG: "#1b1a17", Dim: "#6d6659", Rule: "#cbc4b4", Thumb: "#8d8676",
		Dots: [3]lipgloss.Color{"#d1443c", "#d99a1a", "#3f8f43"},
	},
	"blueprint": {
		Name: "blueprint", BG: "#06151f", FG: "#d3e8f7", Dim: "#5a87a5", Rule: "#1b3b52", Thumb: "#4691bd",
		Dots: [3]lipgloss.Color{"#e0736c", "#d9a84e", "#57b8a9"},
	},
}

// ThemeByName looks up a theme from config. An empty or unknown name yields
// the default and false, so the caller can say so once at startup instead of
// failing a candidate's connection over a typo.
func ThemeByName(name string) (Theme, bool) {
	if name == "" {
		return themes[DefaultThemeName], true
	}
	t, ok := themes[name]
	if !ok {
		return themes[DefaultThemeName], false
	}
	return t, true
}

// ThemeNames lists every available theme, sorted, for error messages.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Styles are all built from one *lipgloss.Renderer, scoped to a single SSH
// session. A style built via the package-level lipgloss.NewStyle() instead
// would render color-blind: lipgloss's global default renderer detects
// color support from the *server process's own* stdout (redirected to a
// log file, not a terminal), not the connecting client's — every SSH client
// would silently get plain, uncolored text regardless of its own terminal.
type Styles struct {
	header, dim, help        lipgloss.Style
	card, titleBar, rule     lipgloss.Style
	scrollThumb, scrollTrack lipgloss.Style
	dots                     [3]lipgloss.Style
}

func newStyles(r *lipgloss.Renderer, t Theme) Styles {
	// Every style carries the theme's background explicitly. Leaving it to
	// the enclosing card style doesn't work: lipgloss terminates each
	// rendered segment with a reset, which also drops the background the
	// card set, so the rest of that line falls back to whatever the viewer's
	// terminal uses — a grey band across the title bar, a stray cell after
	// the selected job. Re-asserting it per style keeps every cell painted.
	base := r.NewStyle().Background(t.BG)

	s := Styles{
		header: base.Bold(true).Foreground(t.FG),
		dim:    base.Foreground(t.Dim),
		help:   base.Foreground(t.Dim),
		card: base.Foreground(t.FG).Padding(cardPadV, cardPadH).
			Border(lipgloss.NormalBorder()).BorderForeground(t.Rule).BorderBackground(t.BG),
		titleBar:    base.Foreground(t.Dim),
		rule:        base.Foreground(t.Rule),
		scrollThumb: base.Foreground(t.Thumb),
		scrollTrack: base.Foreground(t.Rule),
	}
	for i, c := range t.Dots {
		s.dots[i] = base.Foreground(c)
	}
	return s
}

// maxCardWidth is the widest a card may grow, leaving cardMarginX clear on
// each side of the terminal.
func (m Model) maxCardWidth() int {
	if m.width <= 0 {
		return cardWidth
	}
	// cardMarginX is clear space left around the card, so the card's own
	// padding and border have to come out of the content budget too.
	return max(m.width-cardMarginX-cardPadH*2-2, cardMinWidth)
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
	return min(cardWidth, m.maxCardWidth())
}

// Even the gaps get rendered through a style: a bare " " between two styled
// segments inherits the terminal's default background, not the card's.
func (m Model) dots() string {
	rendered := make([]string, len(m.styles.dots))
	for i, style := range m.styles.dots {
		rendered[i] = style.Render("●")
	}
	return strings.Join(rendered, m.styles.dim.Render(" "))
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
	bar := left + m.styles.dim.Render(strings.Repeat(" ", gap)) + right
	rule := m.styles.rule.Render(strings.Repeat("─", innerWidth))
	return bar + "\n" + rule
}

// scrollbar draws the description viewport's position as a one-column bar:
// a thumb sized to the visible fraction of the content, sitting in a dimmer
// full-height track. Returns "" when everything already fits, so a short
// posting shows no bar at all.
func (m Model) scrollbar(height int) string {
	total, visible := m.detail.TotalLineCount(), m.detail.VisibleLineCount()
	if height <= 0 || total <= visible {
		return ""
	}

	thumb := max(height*visible/total, 1)
	offset := 0
	if scrollable := total - visible; scrollable > 0 {
		offset = m.detail.YOffset * (height - thumb) / scrollable
	}

	// The leading gap column is part of every row rather than a separate
	// block joined alongside: a one-row-tall spacer gets padded out to full
	// height by JoinHorizontal with unstyled spaces, which show up as a grey
	// stripe in any terminal whose default background isn't black.
	gap := m.styles.dim.Render(" ")

	rows := make([]string, height)
	for i := range rows {
		style := m.styles.scrollTrack
		if i >= offset && i < offset+thumb {
			style = m.styles.scrollThumb
		}
		rows[i] = gap + style.Render("│")
	}
	return strings.Join(rows, "\n")
}

// renderCard builds a card sized to fit width (the content's own natural
// width, or a fixed prose width — see each view* function), wraps it in the
// window chrome, and centers the result on a black canvas covering the
// whole terminal.
func (m Model) renderCard(title string, width int, body string) string {
	width = max(width, m.titleBarMinWidth(title), cardWidth)
	width = min(max(width, cardMinWidth), m.maxCardWidth())

	content := m.chrome(width, title) + "\n\n" + body
	box := m.styles.card.Width(width + cardPadH*2).Render(content)
	if m.width <= 0 || m.height <= 0 {
		return box
	}
	return m.renderer.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceBackground(m.theme.BG))
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
	style, prefix := d.styles.dim, "  "
	if index == m.Index() {
		style, prefix = d.styles.header, "› "
	}
	// Padded to the full list width so the row's background runs to the edge
	// instead of stopping at the title and leaving bare cells behind it.
	fmt.Fprint(w, style.Width(m.Width()).Render(prefix+item.job.Title))
}

// applyTheme is a minimal, monochrome huh theme matching the card's
// palette, built on huh's own grayscale ThemeBase for structure (border
// shapes, prefix strings, key bindings) with every color-bearing field
// rebuilt from the session's renderer instead of huh's own package-level
// default — the same renderer-binding concern Styles exists for.
func (m Model) applyTheme() *huh.Theme {
	r := m.renderer
	t := huh.ThemeBase()

	base := r.NewStyle().Background(m.theme.BG) // see newStyles: the card's own background doesn't survive nested styles

	t.Focused.Base = base.PaddingLeft(1).
		BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).
		BorderForeground(m.theme.Rule).BorderBackground(m.theme.BG)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = base.Foreground(m.theme.FG).Bold(true)
	t.Focused.Description = base.Foreground(m.theme.Dim)
	t.Focused.TextInput.Text = base.Foreground(m.theme.FG)
	t.Focused.TextInput.Prompt = base.Foreground(m.theme.Dim)
	t.Focused.TextInput.Placeholder = base.Foreground(m.theme.Rule)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.Title = base.Foreground(m.theme.Dim)

	t.Help.ShortKey = base.Foreground(m.theme.Dim)
	t.Help.ShortDesc = base.Foreground(m.theme.Rule)
	t.Help.FullKey = base.Foreground(m.theme.Dim)
	t.Help.FullDesc = base.Foreground(m.theme.Rule)
	return t
}
