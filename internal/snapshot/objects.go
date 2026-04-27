package snapshot

import (
	"archive/tar"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ObjectsOptions describes an S3-compatible source bucket.
type ObjectsOptions struct {
	Bucket string
	Prefix string
	Client *s3.Client // injectable for tests / MinIO
}

// Objects streams a tar of every object under Prefix in Bucket.
type Objects struct {
	opts ObjectsOptions
}

// NewObjects builds an Objects snapshotter.
func NewObjects(opts ObjectsOptions) *Objects { return &Objects{opts: opts} }

// Stream writes a tar archive of all objects in Bucket/Prefix to w.
func (o *Objects) Stream(ctx context.Context, w io.Writer) error {
	if o.opts.Client == nil {
		return fmt.Errorf("objects snapshot: s3 client required")
	}
	tw := tar.NewWriter(w)
	defer tw.Close()

	var token *string
	for {
		out, err := o.opts.Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(o.opts.Bucket),
			Prefix:            aws.String(o.opts.Prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range out.Contents {
			if err := o.appendObject(ctx, tw, aws.ToString(obj.Key), aws.ToInt64(obj.Size)); err != nil {
				return err
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			break
		}
		token = out.NextContinuationToken
	}
	return nil
}

func (o *Objects) appendObject(ctx context.Context, tw *tar.Writer, key string, size int64) error {
	got, err := o.opts.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(o.opts.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get %s: %w", key, err)
	}
	defer got.Body.Close()

	hdr := &tar.Header{Name: key, Mode: 0o644, Size: size}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar header %s: %w", key, err)
	}
	if _, err := io.Copy(tw, got.Body); err != nil {
		return fmt.Errorf("tar copy %s: %w", key, err)
	}
	return nil
}
