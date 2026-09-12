package sink

import (
	"context"

	"github.com/sharadregoti/sshire/internal/model"
	"github.com/sharadregoti/sshire/internal/store"
)

// DashboardSink writes every application to the built-in SQLite store so it
// shows up in the dashboard, with no ATS or webhook required.
type DashboardSink struct {
	store *store.Store
}

func NewDashboardSink(s *store.Store) *DashboardSink {
	return &DashboardSink{store: s}
}

func (d *DashboardSink) Name() string { return "dashboard" }

func (d *DashboardSink) Submit(ctx context.Context, app model.Application) error {
	_, err := d.store.Insert(ctx, app)
	return err
}
