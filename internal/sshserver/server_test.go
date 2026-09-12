package sshserver

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/wish/testsession"
	gossh "golang.org/x/crypto/ssh"

	"github.com/sharadregoti/sshire/internal/config"
	"github.com/sharadregoti/sshire/internal/model"
	"github.com/sharadregoti/sshire/internal/sink"
)

// captureSink records every submitted application so a test can assert the
// full SSH -> pty -> TUI -> apply -> sink path actually delivered one,
// instead of just checking rendered text.
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

func testConfig(tb testing.TB) *config.Config {
	return &config.Config{
		Company: config.Company{Name: "Acme Corp", Tagline: "we build things"},
		SSH:     config.SSH{HostKeyPath: tb.TempDir() + "/host_key"},
		Jobs: []model.Job{
			{ID: "eng-1", Title: "Backend Engineer", Location: "Remote", Type: "Full-time", Description: "Build the thing."},
		},
	}
}

// clientConfig authenticates with a throwaway keypair. The server's
// PublicKeyAuth handler accepts any key by design (it's a public careers
// page, not a login), so any freshly generated key works.
func clientConfig(tb testing.TB) *gossh.ClientConfig {
	tb.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		tb.Fatalf("generate client key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		tb.Fatalf("signer from key: %v", err)
	}
	return &gossh.ClientConfig{
		User:            "candidate",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // test-only, throwaway host key
		Timeout:         5 * time.Second,
	}
}

// TestNoPtyIsRejected proves the "no bots" gate that superlogical.jobs uses
// (confirmed live: piping a non-interactive ssh connection at it returns
// exactly this message) also holds for this server: a client that doesn't
// request a real terminal never reaches the TUI at all.
func TestNoPtyIsRejected(t *testing.T) {
	srv, err := New(testConfig(t), nil)
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	session := testsession.New(t, srv, clientConfig(t))
	out, err := session.CombinedOutput("")
	if err == nil {
		t.Fatal("expected the server to reject a session with no active pty")
	}
	if !strings.Contains(string(out), "Requires an active PTY") {
		t.Fatalf("expected the pty-required message, got %q", out)
	}
}

// TestApplyOverRealSSH drives the whole stack over an actual SSH connection
// with a requested pty: connect, browse to a job, fill out the huh apply
// form by writing raw bytes to stdin as a real terminal would send them, and
// confirm the submission reaches the configured sink.
func TestApplyOverRealSSH(t *testing.T) {
	capture := &captureSink{}
	srv, err := New(testConfig(t), sink.Multi{capture})
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	session := testsession.New(t, srv, clientConfig(t))

	if err := session.RequestPty("xterm-256color", 40, 120, gossh.TerminalModes{}); err != nil {
		t.Fatalf("request pty: %v", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	out := &syncBuffer{}
	go func() { _, _ = out.ReadFrom(stdout) }()

	if err := session.Shell(); err != nil {
		t.Fatalf("start shell: %v", err)
	}

	waitFor(t, out, "Backend Engineer")

	write(t, stdin, "\r") // Enter: select the first (only) job
	waitFor(t, out, "Build the thing.")

	write(t, stdin, "a") // open the apply form
	waitFor(t, out, "Name")

	// huh advances focus to the next field via an async tea.Cmd, so a short
	// pause between fields avoids racing that transition (see the same
	// issue documented in internal/tui's model_test.go).
	fillField := func(value string) {
		write(t, stdin, value+"\r")
		time.Sleep(30 * time.Millisecond)
	}
	fillField("Ada Lovelace")
	fillField("ada@example.com")
	fillField("https://github.com/ada")
	fillField("Excited to apply!")

	waitFor(t, out, "Application submitted")

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(capture.got()) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	apps := capture.got()
	if len(apps) != 1 {
		t.Fatalf("expected exactly 1 application delivered to the sink over SSH, got %d", len(apps))
	}
	got := apps[0]
	if got.JobID != "eng-1" || got.Name != "Ada Lovelace" || got.Email != "ada@example.com" {
		t.Fatalf("unexpected application recorded: %+v", got)
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) ReadFrom(r interface{ Read([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 4096)
	var total int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.buf.Write(buf[:n])
			s.mu.Unlock()
			total += int64(n)
		}
		if err != nil {
			return total, err
		}
	}
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func write(tb testing.TB, w interface{ Write([]byte) (int, error) }, s string) {
	tb.Helper()
	if _, err := w.Write([]byte(s)); err != nil {
		tb.Fatalf("write %q: %v", s, err)
	}
}

func waitFor(tb testing.TB, out *syncBuffer, substr string) {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), substr) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	tb.Fatalf("timed out waiting for %q in ssh output:\n%s", substr, out.String())
}
