package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

type CutterAdapter struct {
	runner *execplatform.Runner
	stdout io.Writer
	stderr io.Writer
}

func NewCutter(runner *execplatform.Runner, stdout, stderr io.Writer) *CutterAdapter {
	return &CutterAdapter{
		runner: runner,
		stdout: stdout,
		stderr: stderr,
	}
}

func (a *CutterAdapter) SplitVideo(
	ctx context.Context,
	src, dir, prefix string,
	segSecs int,
) ([]string, error) {
	pattern := filepath.Join(dir, prefix+"%03d.mp4")
	err := a.runner.Run(ctx, a.stdout, a.stderr, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", src,
		"-an", "-c:v", "copy",
		"-f", "segment", "-segment_time", fmt.Sprint(segSecs),
		"-reset_timestamps", "1",
		pattern, "-y",
	)
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, prefix+"*.mp4"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("нарезка не дала сегментов")
	}
	return matches, nil
}

func (a *CutterAdapter) HasAudio(ctx context.Context, src string) (bool, error) {
	out, err := a.runner.Output(ctx, "ffprobe",
		"-hide_banner", "-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=index",
		"-of", "csv=p=0",
		src,
	)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func (a *CutterAdapter) ExtractAudio(
	ctx context.Context,
	src, dst, codec, bitrate string,
) error {
	return a.runner.Run(ctx, a.stdout, a.stderr, "ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", src,
		"-vn", "-c:a", codec, "-b:a", bitrate,
		dst, "-y",
	)
}

func (a *CutterAdapter) Concat(
	ctx context.Context,
	parts []string,
	audio, out string,
) error {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("file " + concatQuote(p) + "\n")
	}
	f, err := os.CreateTemp("", "qffconcat*.txt")
	if err != nil {
		return err
	}
	listPath := f.Name()
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		os.Remove(listPath)
		return err
	}
	f.Close()
	defer os.Remove(listPath)

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-i", listPath,
	}
	if audio != "" {
		args = append(args, "-i", audio,
			"-c", "copy", "-map", "0:v", "-map", "1:a",
			out, "-y",
		)
	} else {
		args = append(args, "-c", "copy", out, "-y")
	}

	return a.runner.Run(ctx, a.stdout, a.stderr, "ffmpeg", args...)
}

func concatQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
