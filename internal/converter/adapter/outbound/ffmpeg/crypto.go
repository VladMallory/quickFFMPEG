package ffmpeg

import (
	"context"
	"io"

	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

type CryptoEncoderAdapter struct {
	runner *execplatform.Runner
	stdout io.Writer
	stderr io.Writer
	dec    domain.Crypt
	enc    domain.Crypt
}

func NewCryptoEncoder(
	runner *execplatform.Runner,
	stdout, stderr io.Writer,
	dec, enc domain.Crypt,
) *CryptoEncoderAdapter {
	return &CryptoEncoderAdapter{
		runner: runner,
		stdout: stdout,
		stderr: stderr,
		dec:    dec,
		enc:    enc,
	}
}

func (a *CryptoEncoderAdapter) Encode(
	ctx context.Context,
	cfg domain.Config,
	inputPath, outputPath string,
) error {
	args := domain.BuildArgs(cfg, inputPath, outputPath, &a.dec, &a.enc)

	return a.runner.Run(ctx, a.stdout, a.stderr, "ffmpeg", args...)
}

type CryptoProberAdapter struct {
	runner *execplatform.Runner
	keys   []domain.Crypt
}

func NewCryptoProber(runner *execplatform.Runner, keys ...domain.Crypt) *CryptoProberAdapter {
	return &CryptoProberAdapter{runner: runner, keys: keys}
}

func (a *CryptoProberAdapter) Resolution(ctx context.Context, path string) string {
	for _, k := range a.keys {
		out, err := a.runner.Output(
			ctx,
			"ffprobe",
			"-decryption_key", k.Key,
			"-decryption_iv", k.IV,
			"-v", "error",
			"-select_streams", "v:0",
			"-show_entries", "frame=width,height:frame_side_data=side_data_type,rotation",
			"-read_intervals", "%+#1",
			"-of", "default=noprint_wrappers=1:nokey=1",
			"crypto:file://"+path,
		)
		if err != nil {
			continue
		}
		if res := parseResolution(out); res != "N/A" {
			return res
		}
	}
	return "N/A"
}
