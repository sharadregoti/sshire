// Package model holds the domain types shared across the SSH server, the
// TUI, and the sinks that applications get delivered to.
package model

import "time"

type Job struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Location    string `yaml:"location"`
	Type        string `yaml:"type"`
	Description string `yaml:"description"`

	// ApplyEmail, when set, replaces the in-TUI apply form for this job:
	// the detail screen just tells the candidate where to email their
	// application instead. Falls back to Company.ApplyEmail if empty.
	ApplyEmail string `yaml:"apply_email"`
}

type Application struct {
	JobID       string
	JobTitle    string
	Name        string
	Email       string
	ResumeLink  string
	Message     string
	SubmittedAt time.Time
	RemoteAddr  string
}
