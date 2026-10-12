package main

import (
	"context"
	"os"

	"github.com/VladMallory/quickFFMPEG/internal/config"
	"github.com/VladMallory/quickFFMPEG/internal/converter/adapter/inbound/cli"
	"github.com/VladMallory/quickFFMPEG/internal/converter/adapter/outbound/ffmpeg"
	"github.com/VladMallory/quickFFMPEG/internal/converter/adapter/outbound/fs"
	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
	convertersvc "github.com/VladMallory/quickFFMPEG/internal/converter/service"
	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

var BuildTime = "unknown"

type app struct {
	runner *cli.Runner
}

func main() {
	myApp := newApp()
	myApp.run()
}

func newApp() app {
	cfg := config.Load()

	return app{runner: setupConverter(cfg)}
}

func setupConverter(cfg domain.Config) *cli.Runner {
	execRunner := execplatform.New()

	encoder := ffmpeg.NewEncoder(execRunner, os.Stdout, os.Stderr)
	prober := ffmpeg.NewProber(execRunner)
	store := fs.New()

	service := convertersvc.NewConverterService(
		encoder,
		prober,
		store,
	)

	return cli.New(service, cfg, BuildTime, os.Stdout)
}

func (a app) run() {
	a.runner.Run(context.Background())
}
