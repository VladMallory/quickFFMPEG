package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
	convertersvc "github.com/VladMallory/quickFFMPEG/internal/converter/service"
)

type Runner struct {
	service convertersvc.ConverterService
	cfg     domain.Config
	buildTS string
	stdout  io.Writer
}

func New(
	service convertersvc.ConverterService,
	cfg domain.Config,
	buildTS string,
	stdout io.Writer,
) *Runner {
	return &Runner{
		service: service,
		cfg:     cfg,
		buildTS: buildTS,
		stdout:  stdout,
	}
}

func (r *Runner) Run(ctx context.Context) {
	r.printBuildTime()

	stats, err := r.service.ConvertAll(ctx, r.cfg, func(outputPath string) {
		fmt.Fprintf(r.stdout, "✅ Файл успешно сохранен в: %s\n", outputPath)
	})
	if err != nil {
		log.Fatal(err)
	}

	r.printReport(stats)
}

func (r *Runner) RunSingle(ctx context.Context, name, inputPath, outputPath string) {
	r.printBuildTime()

	stat, err := r.service.ConvertFile(ctx, r.cfg, name, inputPath, outputPath, func(out string) {
		fmt.Fprintf(r.stdout, "✅ Файл успешно сохранен в: %s\n", out)
	})
	if err != nil {
		log.Fatal(err)
	}

	r.printReport([]domain.FileStat{stat})
}

func (r *Runner) printReport(stats []domain.FileStat) {
	fmt.Fprintln(r.stdout, "\n🎉 Все файлы обработаны!")
	if len(stats) > 0 {
		fmt.Fprintln(r.stdout, "Статистика сжатия")
		for _, stat := range stats {
			fmt.Fprintf(
				r.stdout,
				"- %s, было: %.1fmb (%s), стало: %.1fmb (%s)\n",
				stat.Name, stat.SizeBefore, stat.ResolutionBefore,
				stat.SizeAfter, stat.ResolutionAfter,
			)
		}
	}

	result, _ := json.MarshalIndent(r.cfg, "", " ")
	fmt.Fprintln(r.stdout, string(result))
}

func (r *Runner) printBuildTime() {
	buildTime := r.buildTS
	if t, err := time.Parse(time.RFC3339, buildTime); err != nil || t.Year() <= 1970 {
		buildTime = "unknown"
		if exe, err := os.Executable(); err == nil {
			if fi, err := os.Stat(exe); err == nil && fi.ModTime().Year() > 1970 {
				buildTime = fi.ModTime().Format(time.RFC3339)
			}
		}
	}
	fmt.Fprintln(r.stdout, "Build time:", buildTime)
	if t, err := time.Parse(time.RFC3339, buildTime); err == nil {
		fmt.Fprintln(r.stdout, "Возраст билда:", time.Since(t).Round(time.Second))
	}
}
