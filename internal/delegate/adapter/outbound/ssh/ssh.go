package sshadapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	delegatedomain "github.com/VladMallory/quickFFMPEG/internal/delegate/domain"
	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

type Adapter struct {
	profile delegatedomain.WorkerProfile
	runner  *execplatform.Runner
	stdout  io.Writer
	stderr  io.Writer
}

func New(
	profile delegatedomain.WorkerProfile,
	runner *execplatform.Runner,
	stdout, stderr io.Writer,
) *Adapter {
	return &Adapter{
		profile: profile,
		runner:  runner,
		stdout:  stdout,
		stderr:  stderr,
	}
}

func (a *Adapter) dst() string {
	return a.profile.User + "@" + a.profile.Host
}

func (a *Adapter) sshBase() []string {
	return []string{
		"-p", fmt.Sprint(a.profile.Port),
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
	}
}

func (a *Adapter) sshOutput(ctx context.Context, remoteCmd string) (string, error) {
	args := append(a.sshBase(), a.dst(), remoteCmd)
	out, err := a.runner.Output(ctx, "ssh", args...)
	return strings.TrimSpace(string(out)), err
}

func (a *Adapter) Home(ctx context.Context) (string, error) {
	return a.sshOutput(ctx, "echo $HOME")
}

func (a *Adapter) UnameM(ctx context.Context) (string, error) {
	return a.sshOutput(ctx, "uname -m")
}

func (a *Adapter) EnsureDir(ctx context.Context, dirs ...string) error {
	q := make([]string, len(dirs))
	for i, d := range dirs {
		q[i] = shQuote(d)
	}
	args := append(a.sshBase(), a.dst(), "mkdir -p "+strings.Join(q, " "))
	return a.runner.Run(ctx, a.stdout, a.stderr, "ssh", args...)
}

func (a *Adapter) HasFile(ctx context.Context, remote string) (bool, error) {
	out, err := a.sshOutput(ctx, "test -f "+shQuote(remote)+" && echo yes || echo no")
	if err != nil {
		return false, err
	}
	return out == "yes", nil
}

func (a *Adapter) Put(ctx context.Context, local, remote string) error {
	return retry(3, func() error {
		args := append(a.scpBase(), local, a.dst()+":"+remote)
		return a.runner.Run(ctx, a.stdout, a.stderr, "scp", args...)
	})
}

func (a *Adapter) Get(ctx context.Context, remote, local string) error {
	return retry(3, func() error {
		args := append(a.scpBase(), a.dst()+":"+remote, local)
		return a.runner.Run(ctx, a.stdout, a.stderr, "scp", args...)
	})
}

func (a *Adapter) scpBase() []string {
	return []string{
		"-P", fmt.Sprint(a.profile.Port),
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "StrictHostKeyChecking=accept-new",
	}
}

func (a *Adapter) Remove(ctx context.Context, remotes ...string) error {
	q := make([]string, len(remotes))
	for i, r := range remotes {
		q[i] = shQuote(r)
	}
	args := append(a.sshBase(), a.dst(), "rm -f "+strings.Join(q, " "))
	return a.runner.Run(ctx, a.stdout, a.stderr, "ssh", args...)
}

func (a *Adapter) RunJob(
	ctx context.Context,
	workdir string,
	env map[string]string,
	bin string,
	args []string,
) error {
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, k)
	}
	sort.Strings(names)

	var b bytes.Buffer
	b.WriteString("cd " + shQuote(workdir) + " &&")
	for _, k := range names {
		b.WriteString(" " + k + "=" + shQuote(env[k]))
	}
	b.WriteString(" " + shQuote(bin))
	for _, arg := range args {
		b.WriteString(" " + shQuote(arg))
	}

	sshArgs := append(a.sshBase(), a.dst(), b.String())
	return a.runner.Run(ctx, a.stdout, a.stderr, "ssh", sshArgs...)
}

func (a *Adapter) CheckFFmpeg(ctx context.Context) error {
	args := append(a.sshBase(), a.dst(), "command -v ffmpeg >/dev/null")
	return a.runner.Run(ctx, a.stdout, a.stderr, "ssh", args...)
}

func (a *Adapter) Sha256(ctx context.Context, remote string) (string, error) {
	return retryValue(3, func() (string, error) {
		out, err := a.sshOutput(ctx, "sha256sum "+shQuote(remote))
		if err != nil {
			return "", err
		}
		fields := strings.Fields(out)
		if len(fields) == 0 {
			return "", fmt.Errorf("пустой вывод sha256sum")
		}
		return fields[0], nil
	})
}

func retry(attempts int, fn func() error) error {
	_, err := retryValue(attempts, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

func retryValue[T any](attempts int, fn func() (T, error)) (T, error) {
	var zero T
	var err error
	var result T
	delay := time.Second
	for i := 0; i < attempts; i++ {
		if result, err = fn(); err == nil {
			return result, nil
		}
		time.Sleep(delay)
		delay *= 2
	}
	return zero, err
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
