package sshadapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
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

func (a *Adapter) EnsureFFmpeg(ctx context.Context) error {
	ok, err := a.checkFFmpeg(ctx)
	if err != nil {
		return fmt.Errorf("воркер недоступен: %v", err)
	}
	if ok {
		return nil
	}
	fmt.Fprintln(a.stdout, "Ставлю ffmpeg на воркере...")
	sudo, err := a.sudoPrefix(ctx)
	if err != nil {
		return err
	}
	managers := []string{"apt-get", "dnf", "yum", "pacman", "apk", "zypper"}
	installs := []string{
		"apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ffmpeg",
		"dnf install -y ffmpeg",
		"yum install -y ffmpeg",
		"pacman -Sy --noconfirm ffmpeg",
		"apk add --no-cache ffmpeg",
		"zypper --non-interactive install ffmpeg",
	}
	for i, mgr := range managers {
		if err := a.runQuiet(ctx, "command -v "+mgr+" >/dev/null"); err != nil {
			continue
		}
		if err := a.runQuiet(ctx, sudo+installs[i]); err != nil {
			continue
		}
		if ok, _ := a.checkFFmpeg(ctx); ok {
			return nil
		}
	}
	if err := a.installStatic(ctx, sudo); err != nil {
		return fmt.Errorf("не удалось поставить ffmpeg на воркере (нужен root или sudo без пароля): %v", err)
	}
	if ok, _ := a.checkFFmpeg(ctx); ok {
		return nil
	}
	return fmt.Errorf("не удалось поставить ffmpeg на воркере (нужен root или sudo без пароля)")
}

func (a *Adapter) checkFFmpeg(ctx context.Context) (bool, error) {
	out, err := a.sshOutput(ctx, "ffmpeg -version 2>/dev/null | head -n 1")
	if err != nil {
		return false, err
	}
	return ffmpegMajor(out) >= 5, nil
}

func ffmpegMajor(line string) int {
	const prefix = "ffmpeg version "
	i := strings.Index(line, prefix)
	if i < 0 {
		return 0
	}
	rest := line[i+len(prefix):]
	start := strings.IndexFunc(rest, func(c rune) bool { return c >= '0' && c <= '9' })
	if start < 0 {
		return 0
	}
	digits := ""
	for _, c := range rest[start:] {
		if c < '0' || c > '9' {
			break
		}
		digits += string(c)
	}
	n, _ := strconv.Atoi(digits)
	return n
}

func (a *Adapter) sudoPrefix(ctx context.Context) (string, error) {
	out, err := a.sshOutput(ctx, "id -u")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "0" {
		return "", nil
	}
	return "sudo -n ", nil
}

func (a *Adapter) installStatic(ctx context.Context, sudo string) error {
	const url = "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz"
	dl := "command -v curl >/dev/null && curl -fsSL -o /tmp/ffstatic.tar.xz " + url +
		" || wget -q -O /tmp/ffstatic.tar.xz " + url
	if err := a.runQuiet(ctx, dl); err != nil {
		return err
	}
	unpack := "tar -xJf /tmp/ffstatic.tar.xz -C /tmp" +
		" && cp /tmp/ffmpeg-*-amd64-static/ffmpeg /usr/local/bin/ffmpeg" +
		" && rm -rf /tmp/ffstatic.tar.xz /tmp/ffmpeg-*-amd64-static"
	return a.runQuiet(ctx, sudo+unpack)
}

func (a *Adapter) runQuiet(ctx context.Context, remoteCmd string) error {
	args := append(a.sshBase(), a.dst(), remoteCmd)
	return a.runner.Run(ctx, io.Discard, a.stderr, "ssh", args...)
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

func (a *Adapter) Load(ctx context.Context) (delegatedomain.WorkerLoad, error) {
	out, err := a.sshOutput(ctx, "grep '^cpu ' /proc/stat; sleep 1; grep '^cpu ' /proc/stat; nproc")
	if err != nil {
		return delegatedomain.WorkerLoad{}, err
	}
	return parseLoad(out)
}

func (a *Adapter) KillJobs(ctx context.Context) error {
	if err := a.runQuiet(ctx, "pkill -f \"quickffmpeg --crypt-i[n]\""); err != nil {
		return err
	}
	return a.runQuiet(ctx, "pkill -f \"crypto:fil[e]://\"")
}

func parseLoad(out string) (delegatedomain.WorkerLoad, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		return delegatedomain.WorkerLoad{}, fmt.Errorf("короткий вывод нагрузки")
	}
	pct, err := cpuUsage(lines[0], lines[1])
	if err != nil {
		return delegatedomain.WorkerLoad{}, err
	}
	cores, err := strconv.Atoi(strings.TrimSpace(lines[2]))
	if err != nil {
		return delegatedomain.WorkerLoad{}, err
	}
	return delegatedomain.WorkerLoad{Pct: pct, Cores: cores}, nil
}

func cpuUsage(first, second string) (int, error) {
	idleA, totalA, err := cpuTotals(first)
	if err != nil {
		return 0, err
	}
	idleB, totalB, err := cpuTotals(second)
	if err != nil {
		return 0, err
	}
	return cpuPct(idleA, totalA, idleB, totalB), nil
}

func cpuTotals(line string) (uint64, uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("битая строка cpu")
	}
	nums := make([]uint64, 0, len(fields)-1)
	for _, f := range fields[1:] {
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return 0, 0, err
		}
		nums = append(nums, n)
	}
	var total uint64
	for _, n := range nums {
		total += n
	}
	idle := nums[3]
	if len(nums) > 4 {
		idle += nums[4]
	}
	return idle, total, nil
}

func cpuPct(idleA, totalA, idleB, totalB uint64) int {
	if totalB <= totalA {
		return 0
	}
	pct := 100 * (1 - float64(idleB-idleA)/float64(totalB-totalA))
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return int(pct)
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
