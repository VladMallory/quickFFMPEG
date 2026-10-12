package delegatesvc

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/VladMallory/quickFFMPEG/internal/converter/adapter/outbound/fs"
	converterdomain "github.com/VladMallory/quickFFMPEG/internal/converter/domain"
	delegatedomain "github.com/VladMallory/quickFFMPEG/internal/delegate/domain"
	"github.com/VladMallory/quickFFMPEG/internal/platform/crypt"
)

type fakeShell struct {
	mu   sync.Mutex
	root string
	jobs []string
}

func (f *fakeShell) mapped(remote string) string {
	return filepath.Join(f.root, "remote", filepath.FromSlash(remote))
}

func (f *fakeShell) Home(ctx context.Context) (string, error) {
	return "/home/test", nil
}

func (f *fakeShell) UnameM(ctx context.Context) (string, error) {
	return "x86_64", nil
}

func (f *fakeShell) EnsureDir(ctx context.Context, dirs ...string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(f.mapped(d), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeShell) HasFile(ctx context.Context, remote string) (bool, error) {
	_, err := os.Stat(f.mapped(remote))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func copyBytes(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func (f *fakeShell) Put(ctx context.Context, local, remote string) error {
	return copyBytes(local, f.mapped(remote))
}

func (f *fakeShell) Get(ctx context.Context, remote, local string) error {
	return copyBytes(f.mapped(remote), local)
}

func (f *fakeShell) Remove(ctx context.Context, remotes ...string) error {
	for _, r := range remotes {
		if err := os.Remove(f.mapped(r)); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeShell) RunJob(
	ctx context.Context,
	workdir string,
	env map[string]string,
	bin string,
	args []string,
) error {
	var in, out string
	for i, a := range args {
		if a == "--crypt-in" && i+1 < len(args) {
			in = args[i+1]
		}
		if a == "--crypt-out" && i+1 < len(args) {
			out = args[i+1]
		}
	}
	if in == "" || out == "" {
		return errNoCrypt
	}
	f.mu.Lock()
	f.jobs = append(f.jobs, in+"->"+out)
	f.mu.Unlock()
	return copyBytes(f.mapped(in), f.mapped(out))
}

var errNoCrypt = errFake("нет crypt-путей")

type errFake string

func (e errFake) Error() string { return string(e) }

func (f *fakeShell) EnsureFFmpeg(ctx context.Context) error {
	return nil
}

func (f *fakeShell) Sha256(ctx context.Context, remote string) (string, error) {
	return crypt.FileSHA256(f.mapped(remote))
}

func (f *fakeShell) Load(ctx context.Context) (delegatedomain.WorkerLoad, error) {
	return delegatedomain.WorkerLoad{Pct: 1, Cores: 1}, nil
}

func (f *fakeShell) KillJobs(ctx context.Context) error {
	return nil
}

func (f *fakeShell) JobCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.jobs)
}

type fakeCutter struct{}

func (fakeCutter) SplitVideo(
	ctx context.Context,
	src, dir, prefix string,
	segSecs int,
) ([]string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	half := len(data) / 2
	paths := []string{
		filepath.Join(dir, prefix+"000.mp4"),
		filepath.Join(dir, prefix+"001.mp4"),
	}
	if err := os.WriteFile(paths[0], data[:half], 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(paths[1], data[half:], 0o644); err != nil {
		return nil, err
	}
	return paths, nil
}

func (fakeCutter) HasAudio(ctx context.Context, src string) (bool, error) {
	return false, nil
}

func (fakeCutter) ExtractAudio(ctx context.Context, src, dst, codec, bitrate string) error {
	return nil
}

func (fakeCutter) Concat(ctx context.Context, parts []string, audio, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	for _, p := range parts {
		data, err := os.ReadFile(p)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := f.Write(data); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

type fakeProber struct{}

func (fakeProber) Resolution(ctx context.Context, path string) string {
	return "1x1"
}

func TestFarmAllGlobalSpread(t *testing.T) {
	srcDir := t.TempDir()
	dataA := []byte(strings.Repeat("A", 1000))
	dataB := []byte(strings.Repeat("B", 100))
	if err := os.WriteFile(filepath.Join(srcDir, "a.mp4"), dataA, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.mp4"), dataB, 0o644); err != nil {
		t.Fatal(err)
	}

	remoteRoot := t.TempDir()
	shellA := &fakeShell{root: remoteRoot}
	shellB := &fakeShell{root: remoteRoot}
	binLocal := filepath.Join(t.TempDir(), "quickffmpeg-test")
	if err := os.WriteFile(binLocal, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	workers := []delegatedomain.WorkerProfile{
		{Name: "w0", Power: 1},
		{Name: "w1", Power: 1},
	}
	svc := NewDelegateService(
		crypt.New(),
		[]RemoteShell{shellA, shellB},
		fs.New(),
		fakeProber{},
		fakeCutter{},
		io.Discard,
		binLocal,
	)
	cfg := converterdomain.Config{Path: srcDir, Farm: true, FarmSeg: 60}

	stats, err := svc.DelegateAll(context.Background(), cfg, workers, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("жди 2 статистики, пришло %d", len(stats))
	}
	if shellA.JobCount() != 2 || shellB.JobCount() != 2 {
		t.Fatalf(
			"жди раздачу 2/2, пришло %d/%d",
			shellA.JobCount(), shellB.JobCount(),
		)
	}
	gotA, err := os.ReadFile(filepath.Join(srcDir, "compressed", "a.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotA) != string(dataA) {
		t.Fatal("склейка a.mp4 не совпала с оригиналом")
	}
	gotB, err := os.ReadFile(filepath.Join(srcDir, "compressed", "b.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotB) != string(dataB) {
		t.Fatal("склейка b.mp4 не совпала с оригиналом")
	}
}
