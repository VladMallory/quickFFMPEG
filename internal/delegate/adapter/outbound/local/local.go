package localadapter

import (
	"context"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	delegatedomain "github.com/VladMallory/quickFFMPEG/internal/delegate/domain"
	"github.com/VladMallory/quickFFMPEG/internal/platform/crypt"
)

type Adapter struct {
	stdout io.Writer
	stderr io.Writer
}

func New(stdout, stderr io.Writer) *Adapter {
	return &Adapter{stdout: stdout, stderr: stderr}
}

func (a *Adapter) Home(ctx context.Context) (string, error) {
	return os.UserHomeDir()
}

func (a *Adapter) UnameM(ctx context.Context) (string, error) {
	return runtime.GOARCH, nil
}

func (a *Adapter) EnsureDir(ctx context.Context, dirs ...string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) HasFile(ctx context.Context, remote string) (bool, error) {
	_, err := os.Stat(remote)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (a *Adapter) Put(ctx context.Context, local, remote string) error {
	return copyFile(local, remote)
}

func (a *Adapter) Get(ctx context.Context, remote, local string) error {
	return copyFile(remote, local)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (a *Adapter) Remove(ctx context.Context, remotes ...string) error {
	for _, r := range remotes {
		if err := os.Remove(r); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) RunJob(
	ctx context.Context,
	workdir string,
	env map[string]string,
	bin string,
	args []string,
) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, k)
	}
	sort.Strings(names)
	full := os.Environ()
	for _, k := range names {
		full = append(full, k+"="+env[k])
	}
	cmd := osexec.CommandContext(ctx, exe, args...)
	cmd.Dir = workdir
	cmd.Env = full
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	return cmd.Run()
}

func (a *Adapter) EnsureFFmpeg(ctx context.Context) error {
	path, err := osexec.LookPath("ffmpeg")
	if err != nil {
		return err
	}
	out, err := osexec.CommandContext(ctx, path, "-version").Output()
	if err != nil {
		return err
	}
	if ffmpegMajor(string(out)) < 5 {
		return fmt.Errorf("нужен ffmpeg 5+")
	}
	return nil
}

func ffmpegMajor(line string) int {
	i := strings.Index(line, "ffmpeg version ")
	if i < 0 {
		return 0
	}
	rest := line[i+len("ffmpeg version "):]
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

func (a *Adapter) Sha256(ctx context.Context, remote string) (string, error) {
	return crypt.FileSHA256(remote)
}

func (a *Adapter) Load(ctx context.Context) (delegatedomain.WorkerLoad, error) {
	if runtime.GOOS == "darwin" {
		return a.loadDarwin(ctx)
	}
	return a.loadLinux()
}

func (a *Adapter) loadDarwin(ctx context.Context) (delegatedomain.WorkerLoad, error) {
	read := func() ([]uint64, error) {
		out, err := osexec.CommandContext(ctx, "sysctl", "-n", "kern.cp_time").Output()
		if err != nil {
			return nil, err
		}
		var nums []uint64
		for _, f := range strings.Fields(string(out)) {
			n, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return nil, err
			}
			nums = append(nums, n)
		}
		if len(nums) < 5 {
			return nil, fmt.Errorf("короткий вывод kern.cp_time")
		}
		return nums, nil
	}
	before, err := read()
	if err != nil {
		return delegatedomain.WorkerLoad{}, err
	}
	time.Sleep(time.Second)
	after, err := read()
	if err != nil {
		return delegatedomain.WorkerLoad{}, err
	}
	var totalA, totalB uint64
	for _, n := range before {
		totalA += n
	}
	for _, n := range after {
		totalB += n
	}
	return delegatedomain.WorkerLoad{
		Pct:   cpuPct(before[4], totalA, after[4], totalB),
		Cores: runtime.NumCPU(),
	}, nil
}

func (a *Adapter) loadLinux() (delegatedomain.WorkerLoad, error) {
	read := func() (uint64, uint64, error) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "cpu ") {
				return cpuTotals(line)
			}
		}
		return 0, 0, fmt.Errorf("нет строки cpu")
	}
	idleA, totalA, err := read()
	if err != nil {
		return delegatedomain.WorkerLoad{}, err
	}
	time.Sleep(time.Second)
	idleB, totalB, err := read()
	if err != nil {
		return delegatedomain.WorkerLoad{}, err
	}
	return delegatedomain.WorkerLoad{
		Pct:   cpuPct(idleA, totalA, idleB, totalB),
		Cores: runtime.NumCPU(),
	}, nil
}

func cpuTotals(line string) (uint64, uint64, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, fmt.Errorf("битая строка cpu")
	}
	var nums []uint64
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

func (a *Adapter) KillJobs(ctx context.Context) error {
	_ = osexec.CommandContext(ctx, "pkill", "-f", "quickffmpeg --crypt-i[n]").Run()
	return osexec.CommandContext(ctx, "pkill", "-f", "crypto:fil[e]://").Run()
}
