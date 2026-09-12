// Package store is the shared SQLite-backed persistence used by the
// dashboard sink (writes) and the dashboard HTTP server (reads). Both sides
// open the same file so "built-in storage" needs no extra moving parts.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/sharadregoti/sshire/internal/model"
)

type Record struct {
	ID int64
	model.Application
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// SQLite serializes writes; one connection avoids "database is locked"
	// under the low concurrency a careers page actually sees.
	db.SetMaxOpenConns(1)

	const schema = `
	CREATE TABLE IF NOT EXISTS applications (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id       TEXT NOT NULL,
		job_title    TEXT NOT NULL,
		name         TEXT NOT NULL,
		email        TEXT NOT NULL,
		resume_link  TEXT NOT NULL,
		message      TEXT NOT NULL,
		submitted_at DATETIME NOT NULL,
		remote_addr  TEXT NOT NULL
	);`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Insert(ctx context.Context, app model.Application) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO applications (job_id, job_title, name, email, resume_link, message, submitted_at, remote_addr)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		app.JobID, app.JobTitle, app.Name, app.Email, app.ResumeLink, app.Message, app.SubmittedAt, app.RemoteAddr,
	)
	if err != nil {
		return 0, fmt.Errorf("insert application: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_id, job_title, name, email, resume_link, message, submitted_at, remote_addr
		FROM applications ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.JobID, &r.JobTitle, &r.Name, &r.Email, &r.ResumeLink, &r.Message, &r.SubmittedAt, &r.RemoteAddr); err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (Record, error) {
	var r Record
	err := s.db.QueryRowContext(ctx, `
		SELECT id, job_id, job_title, name, email, resume_link, message, submitted_at, remote_addr
		FROM applications WHERE id = ?`, id,
	).Scan(&r.ID, &r.JobID, &r.JobTitle, &r.Name, &r.Email, &r.ResumeLink, &r.Message, &r.SubmittedAt, &r.RemoteAddr)
	if err != nil {
		return Record{}, fmt.Errorf("get application %d: %w", id, err)
	}
	return r, nil
}
