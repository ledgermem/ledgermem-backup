package snapshot

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestPostgresStreamRequiresDSN — empty DSN must error out fast, not exec
// pg_dump with whatever default it would otherwise pick up.
func TestPostgresStreamRequiresDSN(t *testing.T) {
	p := NewPostgres(PostgresOptions{})
	err := p.Stream(context.Background(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
	if !strings.Contains(err.Error(), "DSN") {
		t.Fatalf("error mention DSN: %v", err)
	}
}

// TestPostgresStreamInvalidDSN exercises the URL parse guard.
func TestPostgresStreamInvalidDSN(t *testing.T) {
	p := NewPostgres(PostgresOptions{DSN: "://not a url"})
	err := p.Stream(context.Background(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

// TestPostgresStreamMissingBinary covers the case where pg_dump is not on
// PATH — we want a clean error, not a panic. We intentionally use a binary
// name that won't exist and a syntactically-valid DSN.
func TestPostgresStreamMissingBinary(t *testing.T) {
	p := NewPostgres(PostgresOptions{
		DSN:        "postgres://localhost:5432/test",
		PgDumpPath: "/nonexistent/pg_dump_definitely_not_there",
	})
	err := p.Stream(context.Background(), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when binary missing")
	}
}
