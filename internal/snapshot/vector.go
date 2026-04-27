package snapshot

import (
	"context"
	"io"
)

// VectorOptions describes the pgvector source.
//
// pgvector lives in the same Postgres database as the rest of LedgerMem
// state, so we simply delegate to a Postgres snapshotter scoped to the
// vector tables.
type VectorOptions struct {
	PostgresOptions
	// Tables — vector tables to dump. When empty, all tables in the
	// "vectors" schema are dumped.
	Tables []string
}

// Vector takes a snapshot of vector store data.
type Vector struct {
	pg *Postgres
}

// NewVector builds a Vector snapshotter wrapping a Postgres dump.
func NewVector(opts VectorOptions) *Vector {
	pgOpts := opts.PostgresOptions
	for _, t := range opts.Tables {
		pgOpts.ExtraArgs = append(pgOpts.ExtraArgs, "--table="+t)
	}
	if len(opts.Tables) == 0 {
		pgOpts.ExtraArgs = append(pgOpts.ExtraArgs, "--schema=vectors")
	}
	return &Vector{pg: NewPostgres(pgOpts)}
}

// Stream writes the vector dump into w.
func (v *Vector) Stream(ctx context.Context, w io.Writer) error {
	return v.pg.Stream(ctx, w)
}
