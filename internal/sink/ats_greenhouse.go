package sink

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sharadregoti/sshire/internal/model"
)

// GreenhouseSink pushes an application into Greenhouse as a new candidate
// via the Harvest API (https://developers.greenhouse.io/harvest.html).
//
// NOT independently verified against a live Greenhouse account: the field
// names below match Greenhouse's documented "Add Candidate" request shape
// at the time this was written, but Harvest API details do shift between
// accounts/plans. Test against a Greenhouse sandbox before relying on this
// in production, and treat it as the reference implementation to copy when
// wiring up Lever, Ashby, or another ATS behind the same Sink interface.
type GreenhouseSink struct {
	boardToken string
	apiKey     string
	http       *http.Client
}

func NewGreenhouseSink(boardToken, apiKey string) *GreenhouseSink {
	return &GreenhouseSink{
		boardToken: boardToken,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *GreenhouseSink) Name() string { return "greenhouse" }

type greenhouseCandidate struct {
	FirstName    string                  `json:"first_name"`
	LastName     string                  `json:"last_name"`
	EmailAddress []greenhouseEmail       `json:"email_addresses"`
	Applications []greenhouseApplication `json:"applications"`
}

type greenhouseEmail struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

type greenhouseApplication struct {
	JobID int    `json:"job_id,omitempty"`
	Notes string `json:"notes,omitempty"`
}

func (g *GreenhouseSink) Submit(ctx context.Context, app model.Application) error {
	first, last := splitName(app.Name)
	notes := fmt.Sprintf("Submitted via sshire (SSH careers TUI).\nResume/links: %s\nMessage: %s", app.ResumeLink, app.Message)

	payload := greenhouseCandidate{
		FirstName:    first,
		LastName:     last,
		EmailAddress: []greenhouseEmail{{Value: app.Email, Type: "personal"}},
		Applications: []greenhouseApplication{{Notes: notes}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode greenhouse candidate: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://harvest.greenhouse.io/v1/candidates", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build greenhouse request: %w", err)
	}
	req.SetBasicAuth(g.apiKey, "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("On-Behalf-Of", "0") // Greenhouse requires a user id here on most accounts; override via config if needed.

	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("post to greenhouse: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("greenhouse returned status %d", resp.StatusCode)
	}
	return nil
}

func splitName(full string) (first, last string) {
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == ' ' {
			return full[:i], full[i+1:]
		}
	}
	return full, ""
}
