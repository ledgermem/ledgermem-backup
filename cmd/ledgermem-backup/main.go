// Command ledgermem-backup is the LedgerMem backup CLI.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	"github.com/ledgermem/ledgermem-backup/internal/encryption"
	"github.com/ledgermem/ledgermem-backup/internal/schedule"
	"github.com/ledgermem/ledgermem-backup/internal/snapshot"
	"github.com/ledgermem/ledgermem-backup/internal/storage"
)

var version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "ledgermem-backup",
		Short: "Encrypted backup & restore for LedgerMem",
		Long:  `ledgermem-backup snapshots Postgres + pgvector + object storage, encrypts with age, and uploads to S3-compatible storage.`,
	}
	root.Version = version

	root.AddCommand(snapshotCmd(), restoreCmd(), verifyCmd(), scheduleCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func snapshotCmd() *cobra.Command {
	var (
		dsn       string
		recipient string
		bucket    string
		key       string
		s3Region  string
		s3Endpoint string
	)
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Take an encrypted snapshot and upload to S3-compatible storage",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if dsn == "" || recipient == "" || bucket == "" {
				return fmt.Errorf("--dsn, --recipient, and --bucket are required")
			}
			if key == "" {
				// Include a random suffix so two snapshots started inside the
				// same second (e.g. retried CronJob, manual + scheduled run)
				// never overwrite each other.
				suffix := make([]byte, 4)
				if _, err := rand.Read(suffix); err != nil {
					return fmt.Errorf("generate key suffix: %w", err)
				}
				key = fmt.Sprintf("ledgermem/%s-%x.pgdump.age",
					time.Now().UTC().Format("20060102T150405"),
					suffix)
			}

			s3Client, err := newS3Client(ctx, s3Region, s3Endpoint)
			if err != nil {
				return err
			}

			// pg_dump | age-encrypt | s3 PutObject — wired via an io.Pipe.
			pr, pw := io.Pipe()
			go func() {
				enc, err := encryption.NewEncryptingWriter(pw, encryption.EncryptOptions{
					RecipientStrings: []string{recipient},
				})
				if err != nil {
					_ = pw.CloseWithError(err)
					return
				}
				err = snapshot.NewPostgres(snapshot.PostgresOptions{DSN: dsn}).Stream(ctx, enc)
				if cerr := enc.Close(); err == nil {
					err = cerr
				}
				_ = pw.CloseWithError(err)
			}()

			sink := storage.NewS3(storage.S3Options{Client: s3Client, Bucket: bucket, Key: key})
			return sink.Upload(ctx, pr)
		},
	}
	cmd.Flags().StringVar(&dsn, "dsn", "", "Postgres DSN (postgres://...)")
	cmd.Flags().StringVar(&recipient, "recipient", "", "age recipient (age1...) for encryption")
	cmd.Flags().StringVar(&bucket, "bucket", "", "Destination S3 bucket")
	cmd.Flags().StringVar(&key, "key", "", "Destination S3 key (defaults to timestamped key)")
	cmd.Flags().StringVar(&s3Region, "region", "us-east-1", "AWS region (for AWS S3)")
	cmd.Flags().StringVar(&s3Endpoint, "endpoint", "", "S3 endpoint URL (for MinIO / R2 / B2)")
	return cmd
}

func restoreCmd() *cobra.Command {
	var (
		identityFile string
		bucket       string
		key          string
		out          string
		s3Region     string
		s3Endpoint   string
	)
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Download and decrypt a snapshot",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			if identityFile == "" || bucket == "" || key == "" || out == "" {
				return fmt.Errorf("--identity, --bucket, --key, and --out are required")
			}
			id, err := encryption.LoadIdentity(identityFile)
			if err != nil {
				return err
			}
			s3Client, err := newS3Client(ctx, s3Region, s3Endpoint)
			if err != nil {
				return err
			}
			obj, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: ptr(bucket), Key: ptr(key)})
			if err != nil {
				return err
			}
			defer obj.Body.Close()

			dst, err := os.Create(filepath.Clean(out))
			if err != nil {
				return err
			}
			defer dst.Close()

			r, err := age.Decrypt(obj.Body, id)
			if err != nil {
				return err
			}
			_, err = io.Copy(dst, r)
			return err
		},
	}
	cmd.Flags().StringVar(&identityFile, "identity", "", "Path to age identity (private key)")
	cmd.Flags().StringVar(&bucket, "bucket", "", "Source S3 bucket")
	cmd.Flags().StringVar(&key, "key", "", "Source S3 key")
	cmd.Flags().StringVar(&out, "out", "", "Local output file (decrypted dump)")
	cmd.Flags().StringVar(&s3Region, "region", "us-east-1", "AWS region")
	cmd.Flags().StringVar(&s3Endpoint, "endpoint", "", "S3 endpoint URL")
	return cmd
}

func verifyCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a snapshot file is non-empty and well-formed",
		RunE: func(_ *cobra.Command, _ []string) error {
			if path == "" {
				return fmt.Errorf("--file is required")
			}
			fi, err := os.Stat(path)
			if err != nil {
				return err
			}
			if fi.Size() == 0 {
				return fmt.Errorf("snapshot file is empty: %s", path)
			}
			fmt.Printf("ok: %s (%d bytes)\n", path, fi.Size())
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "file", "", "Path to snapshot file")
	return cmd
}

func scheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Install ledgermem-backup as a recurring job",
	}
	var (
		systemdDir string
		unitName   string
		cron       string
		execStart  string
	)
	systemd := &cobra.Command{
		Use:   "systemd",
		Short: "Install a systemd timer (Linux)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return schedule.InstallSystemd(schedule.SystemdOptions{
				UnitName:  unitName,
				Schedule:  cron,
				ExecStart: execStart,
				UnitDir:   systemdDir,
			})
		},
	}
	systemd.Flags().StringVar(&systemdDir, "unit-dir", "/etc/systemd/system", "systemd unit directory")
	systemd.Flags().StringVar(&unitName, "name", "ledgermem-backup", "systemd unit base name")
	systemd.Flags().StringVar(&cron, "on-calendar", "daily", "systemd OnCalendar expression")
	systemd.Flags().StringVar(&execStart, "exec", "/usr/local/bin/ledgermem-backup snapshot", "ExecStart command")

	var (
		k8sName string
		k8sNS   string
		k8sCron string
		k8sImg  string
		k8sEnv  string
	)
	k8s := &cobra.Command{
		Use:   "k8s",
		Short: "Emit a Kubernetes CronJob YAML to stdout",
		RunE: func(_ *cobra.Command, args []string) error {
			return schedule.EmitCronJob(os.Stdout, schedule.CronJobOptions{
				Name:      k8sName,
				Namespace: k8sNS,
				Schedule:  k8sCron,
				Image:     k8sImg,
				Args:      args,
				EnvFrom:   k8sEnv,
			})
		},
	}
	k8s.Flags().StringVar(&k8sName, "name", "ledgermem-backup", "CronJob name")
	k8s.Flags().StringVar(&k8sNS, "namespace", "default", "Namespace")
	k8s.Flags().StringVar(&k8sCron, "schedule", "0 2 * * *", "Cron schedule expression")
	k8s.Flags().StringVar(&k8sImg, "image", "ghcr.io/ledgermem/ledgermem-backup:latest", "Container image")
	k8s.Flags().StringVar(&k8sEnv, "env-from", "", "Optional Secret name to source env from")

	cmd.AddCommand(systemd, k8s)
	return cmd
}

func newS3Client(ctx context.Context, region, endpoint string) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	if endpoint != "" {
		return s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = ptr(endpoint)
			o.UsePathStyle = true
		}), nil
	}
	return s3.NewFromConfig(cfg), nil
}

func ptr[T any](v T) *T { return &v }
