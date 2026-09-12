// Package sink defines where a submitted application goes once a candidate
// finishes the apply form: a webhook, an ATS, a built-in dashboard, or any
// combination — a company enables as many as they want in config.yaml.
package sink

import (
	"context"
	"log"

	"github.com/sharadregoti/sshire/internal/model"
)

type Sink interface {
	Name() string
	Submit(ctx context.Context, app model.Application) error
}

// Multi fans one submission out to every configured sink. A failure in one
// sink is logged but never blocks the others or the candidate's "submitted"
// confirmation — losing a Slack ping shouldn't make an application vanish.
type Multi []Sink

func (m Multi) Submit(ctx context.Context, app model.Application) error {
	for _, s := range m {
		if err := s.Submit(ctx, app); err != nil {
			log.Printf("sink %s: %v", s.Name(), err)
		}
	}
	return nil
}
