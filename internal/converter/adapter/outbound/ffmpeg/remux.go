package ffmpeg

import (
	"context"
	"io"

	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

type MuxerAdapter struct {
	runner *execplatform.Runner
	stdout io.Writer
	stderr io.Writer
}

func NewMuxer(runner *execplatform.Runner, stdout, stderr io.Writer) *MuxerAdapter {
	return &MuxerAdapter{
		runner: runner,
		stdout: stdout,
		stderr: stderr,
	}
}

func (a *MuxerAdapter) Remux(ctx context.Context, inputPath, outputPath string) error {
	return a.runner.Run(
		ctx, a.stdout, a.stderr,
		"ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", inputPath,
		"-c", "copy",
		outputPath, "-y",
	)
}
