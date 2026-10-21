package delegatesvc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	converterdomain "github.com/VladMallory/quickFFMPEG/internal/converter/domain"
	delegatedomain "github.com/VladMallory/quickFFMPEG/internal/delegate/domain"
	"github.com/VladMallory/quickFFMPEG/internal/platform/crypt"
)

type Cipher interface {
	RandHex(n int) (string, error)
	EncryptFile(in, out, keyHex, ivHex string) error
	DecryptFile(in, out, keyHex, ivHex string) error
}

type RemoteShell interface {
	Home(ctx context.Context) (string, error)
	UnameM(ctx context.Context) (string, error)
	EnsureDir(ctx context.Context, dirs ...string) error
	HasFile(ctx context.Context, remote string) (bool, error)
	Put(ctx context.Context, local, remote string) error
	Get(ctx context.Context, remote, local string) error
	Remove(ctx context.Context, remotes ...string) error
	RunJob(ctx context.Context, workdir string, env map[string]string, bin string, args []string) error
	CheckFFmpeg(ctx context.Context) error
	Sha256(ctx context.Context, remote string) (string, error)
}

type SourceStore interface {
	MkdirAll(path string) error
	ListFiles(dir string) ([]string, error)
	IsDir(path string) bool
	VideoDirs(root string) ([]string, error)
	SizeMB(path string) (float64, error)
	SaveSettings(dir string, cfg converterdomain.Config) error
}

type Prober interface {
	Resolution(ctx context.Context, path string) string
}

type Muxer interface {
	Remux(ctx context.Context, inputPath, outputPath string) error
}

type DelegateService struct {
	cipher   Cipher
	shell    RemoteShell
	store    SourceStore
	prober   Prober
	muxer    Muxer
	stdout   io.Writer
	binLocal string
}

func NewDelegateService(
	cipher Cipher,
	shell RemoteShell,
	store SourceStore,
	prober Prober,
	muxer Muxer,
	stdout io.Writer,
	binLocal string,
) DelegateService {
	return DelegateService{
		cipher:   cipher,
		shell:    shell,
		store:    store,
		prober:   prober,
		muxer:    muxer,
		stdout:   stdout,
		binLocal: binLocal,
	}
}

func (s DelegateService) DelegateAll(
	ctx context.Context,
	cfg converterdomain.Config,
	worker delegatedomain.WorkerProfile,
	onEncoded func(outputPath string),
) ([]converterdomain.FileStat, error) {
	dir, err := s.prepare(ctx, worker)
	if err != nil {
		return nil, err
	}

	dirs := []string{cfg.Path}
	if cfg.Recursive {
		dirs, err = s.store.VideoDirs(cfg.Path)
		if err != nil {
			return nil, fmt.Errorf("Ошибка чтения директории: %v", err)
		}
	}

	tmpDir, err := os.MkdirTemp("", "qffenc")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	var stats []converterdomain.FileStat

	runID, err := s.cipher.RandHex(8)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(s.stdout, "Прогон:", runID)

	jobIndex := 0
	for _, currentDir := range dirs {
		compressedDir := filepath.Join(currentDir, "compressed")
		if err := s.store.MkdirAll(compressedDir); err != nil {
			return nil, fmt.Errorf("Не удалось создать папку для сжатых файлов: %v", err)
		}

		if err := s.store.SaveSettings(compressedDir, cfg.ForPath(currentDir)); err != nil {
			return nil, err
		}

		names, err := s.store.ListFiles(currentDir)
		if err != nil {
			return nil, fmt.Errorf("Ошибка чтения директории: %v", err)
		}

		for _, name := range names {
			inputPath := filepath.Join(currentDir, name)

			if s.store.IsDir(inputPath) {
				continue
			}

			ext := strings.ToLower(filepath.Ext(inputPath))
			if !converterdomain.ValidExtensions[ext] {
				continue
			}

			displayName, err := filepath.Rel(cfg.Path, inputPath)
			if err != nil {
				displayName = name
			}

			stat, err := s.delegateFile(
				ctx, cfg, dir, tmpDir, compressedDir, runID,
				jobIndex, displayName, inputPath, onEncoded,
			)
			if err != nil {
				return nil, err
			}
			stats = append(stats, stat)
			jobIndex++
		}
	}

	return stats, nil
}

func (s DelegateService) prepare(ctx context.Context, worker delegatedomain.WorkerProfile) (string, error) {
	machine, err := s.shell.UnameM(ctx)
	if err != nil {
		return "", fmt.Errorf("воркер недоступен: %v", err)
	}
	if !strings.Contains(machine, "x86_64") {
		return "", fmt.Errorf("воркер должен быть linux x86_64, а там %s", machine)
	}

	if err := s.shell.CheckFFmpeg(ctx); err != nil {
		return "", fmt.Errorf("на воркере нет ffmpeg: %v", err)
	}

	home, err := s.shell.Home(ctx)
	if err != nil {
		return "", fmt.Errorf("не удалось узнать домашнюю папку воркера: %v", err)
	}
	dir := worker.Dir
	if strings.HasPrefix(dir, "~/") {
		dir = home + dir[1:]
	}

	if err := s.shell.EnsureDir(ctx,
		path.Join(dir, "in"),
		path.Join(dir, "out"),
		path.Join(dir, "bin"),
	); err != nil {
		return "", err
	}

	binRemote := path.Join(dir, "bin", "quickffmpeg")
	has, err := s.shell.HasFile(ctx, binRemote)
	if err != nil {
		return "", err
	}
	if !has {
		if _, err := os.Stat(s.binLocal); err != nil {
			return "", fmt.Errorf("на воркере нет бинарника, собери: make build-worker")
		}
		fmt.Fprintln(s.stdout, "Заливаю бинарник на воркер:", binRemote)
		if err := s.shell.Put(ctx, s.binLocal, binRemote); err != nil {
			return "", fmt.Errorf("не удалось залить бинарник: %v", err)
		}
	}

	return dir, nil
}

func (s DelegateService) delegateFile(
	ctx context.Context,
	cfg converterdomain.Config,
	dir, tmpDir, compressedDir string,
	runID string,
	i int,
	name, inputPath string,
	onEncoded func(outputPath string),
) (converterdomain.FileStat, error) {
	resBefore := s.prober.Resolution(ctx, inputPath)

	sizeBefore, err := s.store.SizeMB(inputPath)
	if err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось получить статистику файла %v", err)
	}

	keys, err := s.freshKeys()
	if err != nil {
		return converterdomain.FileStat{}, err
	}
	dec := converterdomain.Crypt{Key: keys[0], IV: keys[1]}
	enc := converterdomain.Crypt{Key: keys[2], IV: keys[3]}

	job := fmt.Sprintf("%s-job%03d.enc", runID, i)
	encLocal := filepath.Join(tmpDir, job)
	if err := s.cipher.EncryptFile(inputPath, encLocal, dec.Key, dec.IV); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось зашифровать файл: %v", err)
	}
	wantHash, err := crypt.FileSHA256(encLocal)
	if err != nil {
		return converterdomain.FileStat{}, err
	}

	rIn := "in/" + job
	rOut := "out/" + job
	if err := s.shell.Put(ctx, encLocal, path.Join(dir, rIn)); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось отправить файл на воркер: %v", err)
	}
	gotHash, err := s.shell.Sha256(ctx, path.Join(dir, rIn))
	if err != nil {
		return converterdomain.FileStat{}, err
	}
	if gotHash != wantHash {
		return converterdomain.FileStat{}, fmt.Errorf("контрольная сумма не сошлась после отправки")
	}

	env := delegatedomain.RemoteEnv(dec, enc)
	args := delegatedomain.RemoteArgs(cfg, path.Join(dir, rIn), path.Join(dir, rOut))
	if err := s.shell.RunJob(ctx, dir, env, "bin/quickffmpeg", args); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("ошибка кодирования на воркере: %v", err)
	}

	pulled := filepath.Join(tmpDir, "out-"+job)
	if err := s.shell.Get(ctx, path.Join(dir, rOut), pulled); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось забрать результат: %v", err)
	}
	if err := s.shell.Remove(ctx, path.Join(dir, rIn), path.Join(dir, rOut)); err != nil {
		fmt.Fprintln(s.stdout, "Предупреждение: не удалось зачистить файлы на воркере:", err)
	}

	base := strings.TrimSuffix(name, filepath.Ext(name))
	finalLocal := filepath.Join(compressedDir, base+".mp4")
	fragLocal := filepath.Join(tmpDir, "frag-"+job)
	if err := s.cipher.DecryptFile(pulled, fragLocal, enc.Key, enc.IV); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось расшифровать результат: %v", err)
	}
	if err := s.muxer.Remux(ctx, fragLocal, finalLocal); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось перепаковатъ: %v", err)
	}

	resAfter := s.prober.Resolution(ctx, finalLocal)

	sizeAfter, err := s.store.SizeMB(finalLocal)
	if err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось получить размер сжатого файла %v", err)
	}

	if onEncoded != nil {
		onEncoded(finalLocal)
	}

	return converterdomain.FileStat{
		Name:             name,
		OutputPath:       finalLocal,
		SizeBefore:       sizeBefore,
		SizeAfter:        sizeAfter,
		ResolutionBefore: resBefore,
		ResolutionAfter:  resAfter,
	}, nil
}

func (s DelegateService) freshKeys() ([4]string, error) {
	var keys [4]string
	for i := range keys {
		k, err := s.cipher.RandHex(16)
		if err != nil {
			return keys, fmt.Errorf("не удалось сгенерировать ключ: %v", err)
		}
		keys[i] = k
	}
	return keys, nil
}
