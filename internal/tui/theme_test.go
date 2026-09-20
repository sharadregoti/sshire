package tui

import (
	"io"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestThemesAreComplete guards the failure mode a palette has: a colour left
// unset is the empty string, which lipgloss renders as "inherit whatever was
// there", so the element silently disappears into the background instead of
// erroring. Every field of every registered theme has to be populated.
func TestThemesAreComplete(t *testing.T) {
	for key, theme := range themes {
		if theme.Name != key {
			t.Errorf("theme %q registered under a mismatched name %q", key, theme.Name)
		}

		fields := map[string]lipgloss.Color{
			"BG": theme.BG, "FG": theme.FG, "Dim": theme.Dim,
			"Rule": theme.Rule, "Thumb": theme.Thumb,
		}
		for i, dot := range theme.Dots {
			fields[string(rune('0'+i))+" dot"] = dot
		}

		for field, color := range fields {
			if color == "" {
				t.Errorf("theme %q: %s is unset", key, field)
			}
		}
	}
}

func TestThemeByName(t *testing.T) {
	if _, ok := themes[DefaultThemeName]; !ok {
		t.Fatalf("the default theme %q is not registered", DefaultThemeName)
	}

	// An unset ui.theme is not a misconfiguration, so it resolves quietly.
	if theme, ok := ThemeByName(""); !ok || theme.Name != DefaultThemeName {
		t.Errorf("empty name: got (%q, %v), want (%q, true)", theme.Name, ok, DefaultThemeName)
	}

	// A typo has to be reportable, hence the false, but must still yield a
	// usable theme rather than an empty one that renders invisibly.
	if theme, ok := ThemeByName("phosphorr"); ok || theme.Name != DefaultThemeName {
		t.Errorf("unknown name: got (%q, %v), want (%q, false)", theme.Name, ok, DefaultThemeName)
	}

	if theme, ok := ThemeByName("phosphor"); !ok || theme.Name != "phosphor" {
		t.Errorf("known name: got (%q, %v), want (\"phosphor\", true)", theme.Name, ok)
	}
}

// TestNewStylesUsesTheme proves the palette is actually threaded into the
// styles the frame is built from. A refactor that dropped the wiring — or
// reintroduced a hardcoded colour — would still pass every other test here,
// because a wrong-but-consistent colour renders perfectly happily.
func TestNewStylesUsesTheme(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)

	for _, name := range ThemeNames() {
		theme, _ := ThemeByName(name)
		s := newStyles(r, theme)

		checks := []struct {
			what string
			got  lipgloss.TerminalColor
			want lipgloss.Color
		}{
			{"card background", s.card.GetBackground(), theme.BG},
			{"card border", s.card.GetBorderTopForeground(), theme.Rule},
			{"header foreground", s.header.GetForeground(), theme.FG},
			{"dim foreground", s.dim.GetForeground(), theme.Dim},
			{"rule foreground", s.rule.GetForeground(), theme.Rule},
			{"scroll thumb", s.scrollThumb.GetForeground(), theme.Thumb},
			{"scroll track", s.scrollTrack.GetForeground(), theme.Rule},
		}
		for i, dot := range theme.Dots {
			checks = append(checks, struct {
				what string
				got  lipgloss.TerminalColor
				want lipgloss.Color
			}{"dot", s.dots[i].GetForeground(), dot})
		}

		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("theme %q: %s is %v, want %v", name, c.what, c.got, c.want)
			}
		}

		// Every style must set a background, or lipgloss's trailing reset
		// leaves cells painted in the viewer's terminal colour instead of
		// the theme's — the bug behind the grey title bar and the stray
		// cell after the selected job.
		for what, style := range map[string]lipgloss.Style{
			"header": s.header, "dim": s.dim, "help": s.help,
			"titleBar": s.titleBar, "rule": s.rule,
			"scrollThumb": s.scrollThumb, "scrollTrack": s.scrollTrack,
		} {
			if style.GetBackground() != theme.BG {
				t.Errorf("theme %q: style %q does not carry the theme background", name, what)
			}
		}
	}
}
