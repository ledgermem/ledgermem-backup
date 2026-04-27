// Package schedule installs ledgermem-backup as a recurring job.
//
// Two backends are supported:
//   - systemd timer (Linux hosts) — installs unit + timer files
//   - Kubernetes CronJob — emits YAML to stdout for `kubectl apply -f -`
package schedule

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SystemdOptions installs a systemd timer.
type SystemdOptions struct {
	UnitName  string // e.g. "ledgermem-backup"
	Schedule  string // OnCalendar, e.g. "daily" or "*-*-* 02:00:00"
	ExecStart string // command to run, e.g. "/usr/local/bin/ledgermem-backup snapshot --dest s3://..."
	UnitDir   string // /etc/systemd/system in production; tmp dir in tests
	User      string // optional User= directive
}

// InstallSystemd writes <UnitName>.service and <UnitName>.timer to UnitDir.
// The caller is responsible for `systemctl daemon-reload && systemctl enable
// --now <unit>.timer`.
func InstallSystemd(opts SystemdOptions) error {
	if opts.UnitDir == "" {
		return fmt.Errorf("schedule: UnitDir required")
	}
	if err := os.MkdirAll(opts.UnitDir, 0o755); err != nil {
		return err
	}

	service := fmt.Sprintf(`[Unit]
Description=LedgerMem encrypted backup snapshot

[Service]
Type=oneshot
%sExecStart=%s
`, userLine(opts.User), opts.ExecStart)

	timer := fmt.Sprintf(`[Unit]
Description=LedgerMem encrypted backup timer

[Timer]
OnCalendar=%s
Persistent=true
Unit=%s.service

[Install]
WantedBy=timers.target
`, opts.Schedule, opts.UnitName)

	svcPath := filepath.Join(opts.UnitDir, opts.UnitName+".service")
	timerPath := filepath.Join(opts.UnitDir, opts.UnitName+".timer")
	if err := os.WriteFile(svcPath, []byte(service), 0o644); err != nil {
		return err
	}
	return os.WriteFile(timerPath, []byte(timer), 0o644)
}

func userLine(u string) string {
	if u == "" {
		return ""
	}
	return "User=" + u + "\n"
}

// CronJobOptions emits a Kubernetes CronJob.
type CronJobOptions struct {
	Name      string
	Namespace string
	Schedule  string // standard cron expression
	Image     string
	Args      []string
	EnvFrom   string // optional Secret name to pull env from
}

// EmitCronJob writes a CronJob YAML manifest to w.
func EmitCronJob(w io.Writer, opts CronJobOptions) error {
	if opts.Namespace == "" {
		opts.Namespace = "default"
	}
	if opts.Image == "" {
		opts.Image = "ghcr.io/ledgermem/ledgermem-backup:latest"
	}
	args := strings.Join(quoted(opts.Args), ", ")

	envFrom := ""
	if opts.EnvFrom != "" {
		envFrom = fmt.Sprintf(`              envFrom:
                - secretRef:
                    name: %s
`, opts.EnvFrom)
	}

	tpl := fmt.Sprintf(`apiVersion: batch/v1
kind: CronJob
metadata:
  name: %s
  namespace: %s
spec:
  schedule: "%s"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: backup
              image: %s
              args: [%s]
%s`, opts.Name, opts.Namespace, opts.Schedule, opts.Image, args, envFrom)

	_, err := io.WriteString(w, tpl)
	return err
}

func quoted(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return out
}
