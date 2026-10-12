package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/VladMallory/quickFFMPEG/internal/config"
	convertercli "github.com/VladMallory/quickFFMPEG/internal/converter/adapter/inbound/cli"
	"github.com/VladMallory/quickFFMPEG/internal/converter/adapter/outbound/ffmpeg"
	"github.com/VladMallory/quickFFMPEG/internal/converter/adapter/outbound/fs"
	converterdomain "github.com/VladMallory/quickFFMPEG/internal/converter/domain"
	convertersvc "github.com/VladMallory/quickFFMPEG/internal/converter/service"
	delegatecli "github.com/VladMallory/quickFFMPEG/internal/delegate/adapter/inbound/cli"
	sshadapter "github.com/VladMallory/quickFFMPEG/internal/delegate/adapter/outbound/ssh"
	delegatedomain "github.com/VladMallory/quickFFMPEG/internal/delegate/domain"
	delegatesvc "github.com/VladMallory/quickFFMPEG/internal/delegate/service"
	"github.com/VladMallory/quickFFMPEG/internal/platform/crypt"
	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

var BuildTime = "unknown"

type app struct {
	run func(context.Context)
}

func main() {
	newApp().run(context.Background())
}

func newApp() app {
	cfg := config.Load()

	switch {
	case cfg.CryptIn != "" || cfg.CryptOut != "":
		if cfg.CryptIn == "" || cfg.CryptOut == "" {
			log.Fatal("нужны оба флага: --crypt-in и --crypt-out")
		}
		if cfg.Security != "" {
			log.Fatal("флаги --security и --crypt-in/--crypt-out несовместимы")
		}
		return app{run: setupWorker(cfg)}
	case cfg.Security != "":
		return app{run: setupDelegate(cfg)}
	default:
		return app{run: setupConverter(cfg)}
	}
}

func setupConverter(cfg converterdomain.Config) func(context.Context) {
	execRunner := execplatform.New()

	encoder := ffmpeg.NewEncoder(execRunner, os.Stdout, os.Stderr)
	prober := ffmpeg.NewProber(execRunner)
	store := fs.New()

	service := convertersvc.NewConverterService(encoder, prober, store)
	runner := convertercli.New(service, cfg, BuildTime, os.Stdout)

	return runner.Run
}

func setupWorker(cfg converterdomain.Config) func(context.Context) {
	dec, enc, err := config.LoadKeys()
	if err != nil {
		log.Fatal(err)
	}

	execRunner := execplatform.New()

	encoder := ffmpeg.NewCryptoEncoder(execRunner, os.Stdout, os.Stderr, dec, enc)
	prober := ffmpeg.NewCryptoProber(execRunner, dec, enc)
	store := fs.New()

	service := convertersvc.NewConverterService(encoder, prober, store)
	runner := convertercli.New(service, cfg, BuildTime, os.Stdout)

	name := filepath.Base(cfg.CryptIn)

	inAbs, err := filepath.Abs(cfg.CryptIn)
	if err != nil {
		log.Fatal(err)
	}
	outAbs, err := filepath.Abs(cfg.CryptOut)
	if err != nil {
		log.Fatal(err)
	}

	return func(ctx context.Context) {
		runner.RunSingle(ctx, name, inAbs, outAbs)
	}
}

func setupDelegate(cfg converterdomain.Config) func(context.Context) {
	var names []string
	for _, n := range strings.Split(cfg.Security, ",") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		log.Fatal("пустой --security")
	}
	wpath, err := config.WorkersFile(cfg.Workers)
	if err != nil {
		log.Fatal(err)
	}

	workers := make([]delegatedomain.WorkerProfile, 0, len(names))
	shells := make([]delegatesvc.RemoteShell, 0, len(names))

	execRunner := execplatform.New()

	for _, n := range names {
		worker, err := config.LoadWorkerAt(wpath, n)
		if err != nil {
			log.Fatal(err)
		}
		workers = append(workers, worker)
		shells = append(shells, sshadapter.New(worker, execRunner, os.Stdout, os.Stderr))
	}

	prober := ffmpeg.NewProber(execRunner)
	cutter := ffmpeg.NewCutter(execRunner, os.Stdout, os.Stderr)
	store := fs.New()

	service := delegatesvc.NewDelegateService(
		crypt.New(),
		shells,
		store,
		prober,
		cutter,
		os.Stdout,
		"build/quickffmpeg-linux-amd64",
	)
	runner := delegatecli.New(service, cfg, workers, BuildTime, os.Stdout)

	return runner.Run
}
