// Command sshire runs a self-hosted "apply to jobs over SSH" server:
// `ssh careers.yourcompany.com` drops a candidate into a terminal UI
// instead of a web form, and applications fan out to whichever sinks are
// enabled in config.yaml (webhook/Slack, an ATS, and/or the built-in
// dashboard).
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/sharadregoti/sshire/internal/config"
	"github.com/sharadregoti/sshire/internal/dashboard"
	"github.com/sharadregoti/sshire/internal/sink"
	"github.com/sharadregoti/sshire/internal/sshserver"
	"github.com/sharadregoti/sshire/internal/store"
)

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

	log.Printf("%s careers server listening on %s (ssh %s to apply)", cfg.Company.Name, cfg.SSH.ListenAddr, cfg.SSH.ListenAddr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("ssh server: %v", err)
	}
}
