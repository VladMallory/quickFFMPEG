package ffmpeg

import (
	"context"
	"io"

	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

type EncoderAdapter struct {
	runner *execplatform.Runner
	stdout io.Writer
	stderr io.Writer
}

func NewEncoder(runner *execplatform.Runner, stdout, stderr io.Writer) *EncoderAdapter {
	return &EncoderAdapter{
		runner: runner,
		stdout: stdout,
		stderr: stderr,
	}
}

func (a *EncoderAdapter) Encode(
	ctx context.Context,
	cfg domain.Config,
	inputPath, outputPath string,
) error {

	args := domain.BuildArgs(cfg, inputPath, outputPath)

	return a.runner.Run(ctx, a.stdout, a.stderr, "ffmpeg", args...)
}
