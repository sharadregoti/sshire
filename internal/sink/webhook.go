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

// WebhookSink POSTs a JSON payload to an arbitrary URL. With Slack set to
// true it formats the body as a Slack incoming-webhook message instead, so
// the same sink type covers both "generic webhook" and "Slack" config
// entries.
type WebhookSink struct {
	name  string
	url   string
	slack bool
	http  *http.Client
}

func NewWebhookSink(url string, slack bool) *WebhookSink {
	name := "webhook"
	if slack {
		name = "slack"
	}
	return &WebhookSink{
		name:  name,
		url:   url,
		slack: slack,
		http:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WebhookSink) Name() string { return w.name }

func (w *WebhookSink) Submit(ctx context.Context, app model.Application) error {
	var body []byte
	var err error

	if w.slack {
		text := fmt.Sprintf(
			"*New application:* %s\n*Role:* %s\n*Email:* %s\n*Resume/links:* %s\n*Message:* %s",
			app.Name, app.JobTitle, app.Email, app.ResumeLink, app.Message,
		)
		body, err = json.Marshal(map[string]string{"text": text})
	} else {
		body, err = json.Marshal(app)
	}
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.http.Do(req)
	if err != nil {
		return fmt.Errorf("post to %s: %w", w.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s webhook returned status %d", w.name, resp.StatusCode)
	}
	return nil
}
