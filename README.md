# ledgermem-backup

Encrypted backup & restore for [LedgerMem](https://proofly.dev) — Postgres,
pgvector, and S3-compatible object stores. Snapshots are streamed,
client-side encrypted with [age](https://age-encryption.org), and uploaded
to S3 / MinIO / R2 / B2.

> **Untested backups don't exist.** Run `ledgermem-backup verify` after
> every snapshot, and rehearse a full `restore` against a scratch
> environment at least monthly. A snapshot you have never restored is
> worth zero.

## Install

```sh
go install github.com/ledgermem/ledgermem-backup/cmd/ledgermem-backup@latest
# or, with Docker:
docker pull ghcr.io/ledgermem/ledgermem-backup:latest
```

## Commands

### snapshot — take a backup

```sh
ledgermem-backup snapshot \
  --dsn       'postgres://ledgermem:****@db.internal:5432/ledgermem?sslmode=require' \
  --recipient age1q...your-age-public-key \
  --bucket    my-backups \
  --key       'ledgermem/2026-04-28T0200.pgdump.age' \
  --region    us-east-1
```

Add `--endpoint https://s3.minio.local` to use MinIO / R2 / B2.

### restore — pull, decrypt, verify

```sh
ledgermem-backup restore \
  --identity ./age-identity.txt \
  --bucket   my-backups \
  --key      ledgermem/2026-04-28T0200.pgdump.age \
  --out      /tmp/restored.pgdump

# Then load with pg_restore:
pg_restore --clean --if-exists --no-owner -d "$NEW_DSN" /tmp/restored.pgdump
```

### verify — check a downloaded snapshot

```sh
ledgermem-backup verify --file /tmp/restored.pgdump
```

### schedule — install a recurring job

```sh
# systemd timer (Linux)
sudo ledgermem-backup schedule systemd \
  --name        ledgermem-backup \
  --on-calendar 'daily' \
  --exec        '/usr/local/bin/ledgermem-backup snapshot --dsn ... --bucket ...'

# Kubernetes CronJob
ledgermem-backup schedule k8s \
  --name      ledgermem-backup \
  --namespace ledgermem-system \
  --schedule  '0 2 * * *' \
  --image     ghcr.io/ledgermem/ledgermem-backup:latest \
  --env-from  ledgermem-backup-env \
  -- snapshot --dsn $DSN --recipient $AGE_RECIPIENT --bucket $BUCKET \
  | kubectl apply -f -
```

## Develop

```sh
go test ./...
go build -o bin/ledgermem-backup ./cmd/ledgermem-backup
```

## License

[Apache 2.0](./LICENSE)
