// Package sshserver wires up the wish SSH server: guest-only auth (anyone
// can connect, matching a public careers page), a requirement that the
// client allocate a real pty (which is what filters out most scripted
// "auto-apply" bots — they pipe I/O, they don't open a terminal), and the
// bubbletea TUI as the per-session program.
package sshserver

import (
	"net"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bubbletea "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"

	"github.com/sharadregoti/sshire/internal/config"
	"github.com/sharadregoti/sshire/internal/sink"
	"github.com/sharadregoti/sshire/internal/tui"
)

func New(cfg *config.Config, sinks sink.Multi) (*ssh.Server, error) {
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
		})
		return m, []tea.ProgramOption{tea.WithAltScreen()}
	}

	return wish.NewServer(
		wish.WithAddress(cfg.SSH.ListenAddr),
		wish.WithHostKeyPath(cfg.SSH.HostKeyPath),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true // guest access by design: it's a public careers page, not a login.
		}),
		wish.WithMiddleware(
			bubbletea.Middleware(handler),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
}
