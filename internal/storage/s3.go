// Package storage uploads encrypted snapshot blobs to S3-compatible targets.
package storage

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Options configures an S3 sink. Supports AWS S3, MinIO, R2, B2 — any
// S3-compatible target (the *s3.Client is constructed by the caller).
type S3Options struct {
	Client *s3.Client
	Bucket string
	Key    string

	// SSEAlgorithm — optional server-side encryption header ("AES256" or
	// "aws:kms"). Snapshot is also client-side encrypted via age.
	SSEAlgorithm string
	SSEKMSKeyID  string
}

// S3 is the S3 storage backend.
type S3 struct{ opts S3Options }

// NewS3 builds an S3 storage backend.
func NewS3(opts S3Options) *S3 { return &S3{opts: opts} }

// Upload uploads body as Bucket/Key.
func (s *S3) Upload(ctx context.Context, body io.Reader) error {
	if s.opts.Client == nil {
		return errors.New("s3 storage: client required")
	}
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.opts.Bucket),
		Key:    aws.String(s.opts.Key),
		Body:   body,
	}
	if s.opts.SSEAlgorithm != "" {
		in.ServerSideEncryption = s3types.ServerSideEncryption(s.opts.SSEAlgorithm)
	}
	if s.opts.SSEKMSKeyID != "" {
		in.SSEKMSKeyId = aws.String(s.opts.SSEKMSKeyID)
	}
	_, err := s.opts.Client.PutObject(ctx, in)
	return err
}
