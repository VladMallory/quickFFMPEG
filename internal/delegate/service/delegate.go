package delegatesvc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	EnsureFFmpeg(ctx context.Context) error
	Sha256(ctx context.Context, remote string) (string, error)
	Load(ctx context.Context) (delegatedomain.WorkerLoad, error)
	KillJobs(ctx context.Context) error
}

type SourceStore interface {
	MkdirAll(path string) error
	ListFiles(dir string) ([]string, error)
	IsDir(path string) bool
	SizeMB(path string) (float64, error)
	SaveSettings(dir string, cfg converterdomain.Config) error
}

type Prober interface {
	Resolution(ctx context.Context, path string) string
}

type Cutter interface {
	SplitVideo(ctx context.Context, src, dir, prefix string, segSecs int) ([]string, error)
	HasAudio(ctx context.Context, src string) (bool, error)
	ExtractAudio(ctx context.Context, src, dst, codec, bitrate string) error
	Concat(ctx context.Context, parts []string, audio, out string) error
}

type DelegateService struct {
	cipher   Cipher
	shells   []RemoteShell
	store    SourceStore
	prober   Prober
	cutter   Cutter
	stdout   io.Writer
	binLocal string
}

func NewDelegateService(
	cipher Cipher,
	shells []RemoteShell,
	store SourceStore,
	prober Prober,
	cutter Cutter,
	stdout io.Writer,
	binLocal string,
) DelegateService {
	return DelegateService{
		cipher:   cipher,
		shells:   shells,
		store:    store,
		prober:   prober,
		cutter:   cutter,
		stdout:   stdout,
		binLocal: binLocal,
	}
}

func (s DelegateService) DelegateAll(
	ctx context.Context,
	cfg converterdomain.Config,
	workers []delegatedomain.WorkerProfile,
	onEncoded func(outputPath string),
) ([]converterdomain.FileStat, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("нет воркеров")
	}
	if len(workers) != len(s.shells) {
		return nil, fmt.Errorf("воркеров и подключений поровну быть должно")
	}
	if cfg.Farm && len(workers) < 2 {
		return nil, fmt.Errorf("для --farm нужно минимум два профиля в --security")
	}

	tickCtx, stopTick := context.WithCancel(ctx)
	defer stopTick()
	go s.loadTicker(tickCtx, s.shells, workers)

	type prepared struct {
		idx int
		dir string
		err error
	}
	resCh := make(chan prepared, len(workers))
	var pwg sync.WaitGroup
	for k, w := range workers {
		pwg.Add(1)
		go func(k int, w delegatedomain.WorkerProfile) {
			defer pwg.Done()
			dir, err := s.prepare(ctx, s.shells[k], w)
			resCh <- prepared{idx: k, dir: dir, err: err}
		}(k, w)
	}
	pwg.Wait()
	close(resCh)
	byIdx := make(map[int]prepared, len(workers))
	for r := range resCh {
		byIdx[r.idx] = r
	}
	dirs := make([]string, 0, len(workers))
	shells := make([]RemoteShell, 0, len(workers))
	alive := make([]delegatedomain.WorkerProfile, 0, len(workers))
	var lastErr error
	for k, w := range workers {
		r := byIdx[k]
		if r.err != nil {
			fmt.Fprintf(s.stdout, "Воркер %s недоступен, пропускаю: %v\n", w.Name, r.err)
			lastErr = r.err
			continue
		}
		dirs = append(dirs, r.dir)
		shells = append(shells, s.shells[k])
		alive = append(alive, w)
	}
	if len(alive) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("нет воркеров")
		}
		return nil, lastErr
	}
	workers = alive

	compressedDir := filepath.Join(cfg.Path, "compressed")
	if err := s.store.MkdirAll(compressedDir); err != nil {
		return nil, fmt.Errorf("Не удалось создать папку для сжатых файлов: %v", err)
	}

	if err := s.store.SaveSettings(compressedDir, cfg); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "qffenc")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	names, err := s.store.ListFiles(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("Ошибка чтения директории: %v", err)
	}

	var stats []converterdomain.FileStat

	type fileJob struct {
		name string
		path string
	}
	var jobs []fileJob
	for _, name := range names {
		inputPath := filepath.Join(cfg.Path, name)

		if s.store.IsDir(inputPath) {
			continue
		}

		ext := strings.ToLower(filepath.Ext(inputPath))
		if !converterdomain.ValidExtensions[ext] {
			continue
		}
		jobs = append(jobs, fileJob{name: name, path: inputPath})
	}

	sizes := make([]int64, len(jobs))
	for i, job := range jobs {
		fi, err := os.Stat(job.path)
		if err != nil {
			return nil, fmt.Errorf("не удалось получить статистику файла %v", err)
		}
		sizes[i] = fi.Size()
	}
	powers := make([]int, len(workers))
	for k, w := range workers {
		powers[k] = w.Power
	}
	assign := delegatedomain.AssignSegments(sizes, powers)
	queues := make([][]int, len(workers))
	for i, k := range assign {
		queues[k] = append(queues[k], i)
	}

	results := make([]converterdomain.FileStat, len(jobs))
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, len(jobs))

	for k := range workers {
		queue := queues[k]
		if len(queue) == 0 {
			continue
		}
		wg.Add(1)
		go func(k int, queue []int) {
			defer wg.Done()
			for _, qi := range queue {
				select {
				case <-ctx2.Done():
					return
				default:
				}
				job := jobs[qi]
				var stat converterdomain.FileStat
				var err error
				if cfg.Farm {
					stat, err = s.farmFile(
						ctx2, cfg, workers, shells, dirs,
						tmpDir, compressedDir, qi, job.name, job.path,
					)
				} else {
					base := strings.TrimSuffix(job.name, filepath.Ext(job.name))
					finalPath := filepath.Join(compressedDir, base+".mp4")
					stat, err = s.delegateFile(
						ctx2, cfg, shells[k], dirs[k], tmpDir,
						fmt.Sprintf("job%03d", qi), job.name, job.path, finalPath,
					)
				}
				if err != nil {
					errCh <- fmt.Errorf("%s: %v", job.name, err)
					cancel()
					return
				}
				results[qi] = stat
				if onEncoded != nil {
					onEncoded(stat.OutputPath)
				}
			}
		}(k, queue)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}
	for _, stat := range results {
		stats = append(stats, stat)
	}

	return stats, nil
}

func (s DelegateService) prepare(
	ctx context.Context,
	shell RemoteShell,
	worker delegatedomain.WorkerProfile,
) (string, error) {
	start := time.Now()
	machine, err := shell.UnameM(ctx)
	if err != nil {
		return "", fmt.Errorf("воркер недоступен: %v", err)
	}
	if !worker.Local && !strings.Contains(machine, "x86_64") {
		return "", fmt.Errorf("воркер должен быть linux x86_64, а там %s", machine)
	}

	if err := shell.EnsureFFmpeg(ctx); err != nil {
		return "", err
	}

	home, err := shell.Home(ctx)
	if err != nil {
		return "", fmt.Errorf("не удалось узнать домашнюю папку воркера: %v", err)
	}
	dir := worker.Dir
	if strings.HasPrefix(dir, "~/") {
		dir = home + dir[1:]
	}

	if err := shell.EnsureDir(ctx,
		path.Join(dir, "in"),
		path.Join(dir, "out"),
		path.Join(dir, "bin"),
	); err != nil {
		return "", err
	}

	binRemote := path.Join(dir, "bin", "quickffmpeg")
	if !worker.Local {
		has, err := shell.HasFile(ctx, binRemote)
		if err != nil {
			return "", err
		}
		if !has {
			if _, err := os.Stat(s.binLocal); err != nil {
				return "", fmt.Errorf("на воркере нет бинарника, собери: make build-worker")
			}
			fmt.Fprintln(s.stdout, "Заливаю бинарник на воркер:", binRemote)
			if err := shell.Put(ctx, s.binLocal, binRemote); err != nil {
				return "", fmt.Errorf("не удалось залить бинарник: %v", err)
			}
		}
	}

	fmt.Fprintf(s.stdout, "Воркер %s готов за %s\n", worker.Name, time.Since(start).Round(time.Second))
	return dir, nil
}

func (s DelegateService) delegateFile(
	ctx context.Context,
	cfg converterdomain.Config,
	shell RemoteShell,
	dir, tmpDir, tag, name, inputPath, finalPath string,
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

	encLocal := filepath.Join(tmpDir, tag+".enc")
	if err := s.cipher.EncryptFile(inputPath, encLocal, dec.Key, dec.IV); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось зашифровать файл: %v", err)
	}
	wantHash, err := crypt.FileSHA256(encLocal)
	if err != nil {
		return converterdomain.FileStat{}, err
	}

	rIn := "in/" + tag + ".enc"
	rOut := "out/" + tag + ".enc"
	if err := shell.Put(ctx, encLocal, path.Join(dir, rIn)); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось отправить файл на воркер: %v", err)
	}
	gotHash, err := shell.Sha256(ctx, path.Join(dir, rIn))
	if err != nil {
		return converterdomain.FileStat{}, err
	}
	if gotHash != wantHash {
		return converterdomain.FileStat{}, fmt.Errorf("контрольная сумма не сошлась после отправки")
	}

	env := delegatedomain.RemoteEnv(dec, enc)
	args := delegatedomain.RemoteArgs(cfg, path.Join(dir, rIn), path.Join(dir, rOut))
	if err := shell.RunJob(ctx, dir, env, "bin/quickffmpeg", args); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("ошибка кодирования на воркере: %v", err)
	}

	pulled := filepath.Join(tmpDir, "out-"+tag+".enc")
	if err := shell.Get(ctx, path.Join(dir, rOut), pulled); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось забрать результат: %v", err)
	}
	if err := shell.Remove(ctx, path.Join(dir, rIn), path.Join(dir, rOut)); err != nil {
		fmt.Fprintln(s.stdout, "Предупреждение: не удалось зачистить файлы на воркере:", err)
	}

	if err := s.cipher.DecryptFile(pulled, finalPath, enc.Key, enc.IV); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось расшифровать результат: %v", err)
	}

	resAfter := s.prober.Resolution(ctx, finalPath)

	sizeAfter, err := s.store.SizeMB(finalPath)
	if err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось получить размер сжатого файла %v", err)
	}

	return converterdomain.FileStat{
		Name:             name,
		OutputPath:       finalPath,
		SizeBefore:       sizeBefore,
		SizeAfter:        sizeAfter,
		ResolutionBefore: resBefore,
		ResolutionAfter:  resAfter,
	}, nil
}

func (s DelegateService) farmFile(
	ctx context.Context,
	cfg converterdomain.Config,
	workers []delegatedomain.WorkerProfile,
	shells []RemoteShell,
	dirs []string,
	tmpDir, compressedDir string,
	i int,
	name, inputPath string,
) (converterdomain.FileStat, error) {
	resBefore := s.prober.Resolution(ctx, inputPath)

	sizeBefore, err := s.store.SizeMB(inputPath)
	if err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось получить статистику файла %v", err)
	}

	segDir := filepath.Join(tmpDir, fmt.Sprintf("farm%03d", i))
	if err := os.MkdirAll(segDir, 0o755); err != nil {
		return converterdomain.FileStat{}, err
	}

	segSecs := cfg.FarmSeg
	if segSecs <= 0 {
		segSecs = 60
	}
	segs, err := s.cutter.SplitVideo(ctx, inputPath, segDir, "seg", segSecs)
	if err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось нарезать видео: %v", err)
	}

	sizes := make([]int64, len(segs))
	for j, segPath := range segs {
		fi, err := os.Stat(segPath)
		if err != nil {
			return converterdomain.FileStat{}, err
		}
		sizes[j] = fi.Size()
	}
	powers := make([]int, len(workers))
	for k, w := range workers {
		powers[k] = w.Power
	}
	assign := delegatedomain.AssignSegments(sizes, powers)
	queues := make([][]int, len(workers))
	for j, k := range assign {
		queues[k] = append(queues[k], j)
	}
	for k, w := range workers {
		if len(queues[k]) > 0 {
			fmt.Fprintf(s.stdout, "Ферма: %s берет куски %v\n", w.Name, queues[k])
		}
	}

	audioLocal := ""
	hasAudio, err := s.cutter.HasAudio(ctx, inputPath)
	if err == nil && hasAudio {
		audioLocal = filepath.Join(segDir, "audio.mkv")
		if err := s.cutter.ExtractAudio(
			ctx, inputPath, audioLocal, cfg.AudioCodecValue, cfg.AudioBitrate,
		); err != nil {
			return converterdomain.FileStat{}, fmt.Errorf("не удалось вытащить аудио: %v", err)
		}
	}

	results := make([]string, len(segs))
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, len(segs))

	for k := range workers {
		queue := queues[k]
		if len(queue) == 0 {
			continue
		}
		slots := workers[k].Slots
		if slots <= 0 {
			slots = 1
		}
		wg.Add(1)
		go func(k int, queue []int) {
			defer wg.Done()
			sem := make(chan struct{}, slots)
			var inner sync.WaitGroup
			for _, j := range queue {
				select {
				case <-ctx2.Done():
					return
				default:
				}
				sem <- struct{}{}
				inner.Add(1)
				go func(j int) {
					defer inner.Done()
					defer func() { <-sem }()
					segStart := time.Now()
					tag := fmt.Sprintf("job%03ds%02d", i, j)
					final := filepath.Join(segDir, fmt.Sprintf("dec%02d.mp4", j))
					_, err := s.delegateFile(
						ctx2, cfg, shells[k], dirs[k], tmpDir,
						tag, filepath.Base(segs[j]), segs[j], final,
					)
					if err != nil {
						errCh <- fmt.Errorf("сегмент %d: %v", j, err)
						cancel()
						return
					}
					results[j] = final
					fmt.Fprintf(
						s.stdout, "Ферма: кусок %d готов за %s (%s)\n",
						j, time.Since(segStart).Round(time.Second), workers[k].Name,
					)
				}(j)
			}
			inner.Wait()
		}(k, queue)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return converterdomain.FileStat{}, err
		}
	}

	base := strings.TrimSuffix(name, filepath.Ext(name))
	finalLocal := filepath.Join(compressedDir, base+".mp4")
	if err := s.cutter.Concat(ctx, results, audioLocal, finalLocal); err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось склеить: %v", err)
	}

	resAfter := s.prober.Resolution(ctx, finalLocal)

	sizeAfter, err := s.store.SizeMB(finalLocal)
	if err != nil {
		return converterdomain.FileStat{}, fmt.Errorf("не удалось получить размер сжатого файла %v", err)
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

func (s DelegateService) CancelRemote() {
	fmt.Fprintln(s.stdout, "Останавливаю воркеры...")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, shell := range s.shells {
		_ = shell.KillJobs(ctx)
	}
}

func (s DelegateService) loadTicker(
	ctx context.Context,
	shells []RemoteShell,
	workers []delegatedomain.WorkerProfile,
) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	s.reportLoad(ctx, shells, workers)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.reportLoad(ctx, shells, workers)
		}
	}
}

func (s DelegateService) reportLoad(
	ctx context.Context,
	shells []RemoteShell,
	workers []delegatedomain.WorkerProfile,
) {
	type res struct {
		idx  int
		load delegatedomain.WorkerLoad
		err  error
	}
	ch := make(chan res, len(workers))
	for k := range workers {
		go func(k int) {
			load, err := shells[k].Load(ctx)
			ch <- res{idx: k, load: load, err: err}
		}(k)
	}
	lines := make([]string, len(workers))
	for range workers {
		r := <-ch
		if r.err != nil {
			lines[r.idx] = workers[r.idx].Name + ": ?"
		} else {
			lines[r.idx] = fmt.Sprintf(
				"%s: %s/%d", workers[r.idx].Name, r.load.One, r.load.Cores,
			)
		}
	}
	fmt.Fprintf(s.stdout, "Нагрузка: %s\n", strings.Join(lines, " | "))
}
