// Package snapshot streams point-in-time snapshots of Mnemo state.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
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
	parsed, err := url.Parse(p.opts.DSN)
	if err != nil {
		return fmt.Errorf("postgres snapshot: invalid DSN: %w", err)
	}

	// Pass the DSN via PGPASSWORD / a sanitized URI in the environment so
	// the password never appears in the process argv (visible to anyone
	// who can read /proc on the host or to `ps`).
	env := append([]string{}, os.Environ()...)
	dsnForArg := p.opts.DSN
	if parsed.User != nil {
		if pw, ok := parsed.User.Password(); ok && pw != "" {
			env = append(env, "PGPASSWORD="+pw)
			// Strip the password from the URI we hand to pg_dump.
			parsed.User = url.User(parsed.User.Username())
			dsnForArg = parsed.String()
		}
	}

	args := append([]string{"--no-owner", "--no-privileges", "--format=custom", dsnForArg}, p.opts.ExtraArgs...)
	cmd := exec.CommandContext(ctx, p.opts.PgDumpPath, args...)
	cmd.Env = env
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
