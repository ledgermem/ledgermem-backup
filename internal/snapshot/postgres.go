// Package snapshot streams point-in-time snapshots of LedgerMem state.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
)

// PostgresOptions describes a Postgres source.
type PostgresOptions struct {
	// DSN is a libpq-style connection string, e.g.
	//   postgres://user:pass@host:5432/db?sslmode=require
	DSN string

	// PgDumpPath overrides the pg_dump binary lookup ("pg_dump" by default).
	PgDumpPath string

	// ExtraArgs are appended verbatim — useful for --schema-only,
	// --exclude-table-data, etc.
	ExtraArgs []string
}

// Postgres takes a logical snapshot via pg_dump.
type Postgres struct {
	opts PostgresOptions
}

// NewPostgres builds a Postgres snapshotter.
func NewPostgres(opts PostgresOptions) *Postgres {
	if opts.PgDumpPath == "" {
		opts.PgDumpPath = "pg_dump"
	}
	return &Postgres{opts: opts}
}

// Stream runs pg_dump and copies its stdout into w. The command exits with
// non-zero status on any error and returns it.
func (p *Postgres) Stream(ctx context.Context, w io.Writer) error {
	if p.opts.DSN == "" {
		return errors.New("postgres snapshot: DSN required")
	}
	if _, err := url.Parse(p.opts.DSN); err != nil {
		return fmt.Errorf("postgres snapshot: invalid DSN: %w", err)
	}

	args := append([]string{"--no-owner", "--no-privileges", "--format=custom", p.opts.DSN}, p.opts.ExtraArgs...)
	cmd := exec.CommandContext(ctx, p.opts.PgDumpPath, args...)
	cmd.Stdout = w
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("postgres snapshot: start: %w", err)
	}
	stderrBytes, _ := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("postgres snapshot: %w (%s)", err, string(stderrBytes))
	}
	return nil
}
