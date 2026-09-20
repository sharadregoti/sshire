// Package sshserver wires up the wish SSH server: guest-only auth (anyone
// can connect, matching a public careers page), a requirement that the
// client allocate a real pty (which is what filters out most scripted
// "auto-apply" bots — they pipe I/O, they don't open a terminal), and the
// bubbletea TUI as the per-session program.
package sshserver

import (
	"log"
	"net"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bubbletea "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"

	"github.com/sharadregoti/sshire/internal/config"
	"github.com/sharadregoti/sshire/internal/sink"
	"github.com/sharadregoti/sshire/internal/tui"
)

func New(cfg *config.Config, sinks sink.Multi) (*ssh.Server, error) {
	// Resolved once here rather than per session: a typo should be reported
	// at startup, not logged again on every candidate's connection.
	theme, ok := tui.ThemeByName(cfg.UI.Theme)
	if !ok {
		log.Printf("unknown ui.theme %q, falling back to %q (available: %s)",
			cfg.UI.Theme, tui.DefaultThemeName, strings.Join(tui.ThemeNames(), ", "))
	}

	handler := func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		remote := s.RemoteAddr().String()
		if host, _, err := net.SplitHostPort(remote); err == nil {
			remote = host
		}
		m := tui.New(tui.Options{
			Company:           cfg.Company.Name,
			Tagline:           cfg.Company.Tagline,
			DefaultApplyEmail: cfg.Company.ApplyEmail,
			Jobs:              cfg.Jobs,
			Sinks:             sinks,
			RemoteAddr:        remote,
			Theme:             theme,
			// Session-scoped: reflects this client's actual terminal, not
			// the server process's own (backgrounded, non-tty) stdout.
			Renderer: sessionRenderer(s),
		})
		return m, []tea.ProgramOption{tea.WithAltScreen()}
	}

	return wish.NewServer(
		wish.WithAddress(cfg.SSH.ListenAddr),
		wish.WithHostKeyPath(cfg.SSH.HostKeyPath),
		// No PasswordHandler/PublicKeyHandler/KeyboardInteractiveHandler at
		// all: charmbracelet/ssh then sets NoClientAuth, so any client gets
		// in with zero auth — a candidate with no SSH keypair on their
		// machine still connects. A PublicKeyHandler that just returns true
		// looks equivalent but isn't: it still forces the client to possess
		// and prove *some* key, which fails for anyone with none.
		wish.WithMiddleware(
			// bubbletea.Middleware's plain form silently forces every
			// session down to termenv.Ascii (no color at all), despite its
			// doc comment implying that's a floor — MakeRenderer downgrades
			// whenever the client profile is *higher* than what's passed.
			// Passing TrueColor as the ceiling means "never downgrade";
			// each client still only gets what its own terminal supports.
			bubbletea.MiddlewareWithColorProfile(handler, termenv.TrueColor),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
}

// sessionRenderer builds a *lipgloss.Renderer bound to this session's I/O
// and TERM, so styling reflects the connecting client's real terminal.
//
// Deliberately not wish/bubbletea's own MakeRenderer: for a session without
// an allocated OS pty (our default — see PtyCallback/EmulatePtyCallback),
// that helper also fires an OSC background-color query at the client and
// blocks up to a second waiting for a reply, to auto-detect light vs. dark
// mode. This app doesn't adapt to that (the card is deliberately always
// dark), so the query is pure downside: added latency, and a real risk of
// swallowing a fast typist's first keystrokes while it waits.
func sessionRenderer(s ssh.Session) *lipgloss.Renderer {
	pty, _, ok := s.Pty()
	if !ok || pty.Term == "" || pty.Term == "dumb" {
		return lipgloss.NewRenderer(s, termenv.WithProfile(termenv.Ascii))
	}
	env := append(append([]string{}, s.Environ()...), "TERM="+pty.Term)
	return lipgloss.NewRenderer(s, termenv.WithEnvironment(sshEnviron(env)), termenv.WithUnsafe())
}

// sshEnviron adapts a []string environment (KEY=value entries) to
// termenv.Environ.
type sshEnviron []string

func (e sshEnviron) Environ() []string { return e }

func (e sshEnviron) Getenv(key string) string {
	for _, v := range e {
		if rest, ok := strings.CutPrefix(v, key+"="); ok {
			return rest
		}
	}
	return ""
}
