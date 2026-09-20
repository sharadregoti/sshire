// Command sshire runs a self-hosted "apply to jobs over SSH" server:
// `ssh careers.yourcompany.com` drops a candidate into a terminal UI
// instead of a web form, and applications fan out to whichever sinks are
// enabled in config.yaml (webhook/Slack, an ATS, and/or the built-in
// dashboard).
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/ssh"

	"github.com/sharadregoti/sshire/internal/config"
	"github.com/sharadregoti/sshire/internal/dashboard"
	"github.com/sharadregoti/sshire/internal/sink"
	"github.com/sharadregoti/sshire/internal/sshserver"
	"github.com/sharadregoti/sshire/internal/store"
)

// shutdownGrace is how long connected candidates get to finish before their
// sessions are cut. Kept well under `docker stop`'s own 10s SIGKILL timer so
// the container exits on its own terms rather than being killed mid-write.
const shutdownGrace = 5 * time.Second

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	var sinks sink.Multi

	if cfg.Sinks.Webhook != nil && cfg.Sinks.Webhook.Enabled {
		sinks = append(sinks, sink.NewWebhookSink(cfg.Sinks.Webhook.URL, false))
	}
	if cfg.Sinks.Slack != nil && cfg.Sinks.Slack.Enabled {
		sinks = append(sinks, sink.NewWebhookSink(cfg.Sinks.Slack.URL, true))
	}
	if cfg.Sinks.ATS != nil && cfg.Sinks.ATS.Enabled {
		switch cfg.Sinks.ATS.Provider {
		case "greenhouse":
			sinks = append(sinks, sink.NewGreenhouseSink(cfg.Sinks.ATS.BoardToken, cfg.Sinks.ATS.APIKey))
		default:
			log.Fatalf("unsupported ats provider: %s", cfg.Sinks.ATS.Provider)
		}
	}
	if cfg.Sinks.Dashboard != nil && cfg.Sinks.Dashboard.Enabled {
		st, err := store.Open(cfg.Sinks.Dashboard.DBPath)
		if err != nil {
			log.Fatalf("open dashboard store: %v", err)
		}
		defer st.Close()

		sinks = append(sinks, sink.NewDashboardSink(st))

		dash := dashboard.New(st, cfg.Sinks.Dashboard.AdminUser, cfg.Sinks.Dashboard.AdminPassword)
		go func() {
			if err := dash.ListenAndServe(cfg.Sinks.Dashboard.ListenAddr); err != nil && err != http.ErrServerClosed {
				log.Fatalf("dashboard server: %v", err)
			}
		}()
	}

	if len(sinks) == 0 {
		log.Println("warning: no sinks enabled — applications will be accepted and discarded")
	}

	srv, err := sshserver.New(cfg, sinks)
	if err != nil {
		log.Fatalf("build ssh server: %v", err)
	}

	// Without a handler of our own the process ignores SIGTERM outright
	// whenever a candidate is connected: bubbletea registers a
	// process-wide SIGINT/SIGTERM handler for each live session, which
	// consumes the signal and quits only that session. `docker stop` would
	// then stall for its full timeout and SIGKILL us mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		log.Println("shutting down")

		// Bounded, then forced. Shutdown alone waits for connections to
		// close, and a candidate sitting on the job list never closes one.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown timed out, closing active sessions: %v", err)
			srv.Close()
		}
	}()

	log.Printf("%s careers server listening on %s (ssh %s to apply)", cfg.Company.Name, cfg.SSH.ListenAddr, cfg.SSH.ListenAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Fatalf("ssh server: %v", err)
	}
}
