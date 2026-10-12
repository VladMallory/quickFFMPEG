package ffmpeg

import (
	"context"
	"strings"

	execplatform "github.com/VladMallory/quickFFMPEG/internal/platform/exec"
)

type ProberAdapter struct {
	runner *execplatform.Runner
}

func NewProber(runner *execplatform.Runner) *ProberAdapter {
	return &ProberAdapter{runner: runner}
}

func (a *ProberAdapter) Resolution(ctx context.Context, path string) string {
	out, err := a.runner.Output(
		ctx,
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "frame=width,height:frame_side_data=side_data_type,rotation",
		"-read_intervals", "%+#1",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	if err != nil {
		return "N/A"
	}

	return parseResolution(out)
}

func parseResolution(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "N/A"
	}

	w, h := lines[0], lines[1]

	if len(lines) >= 4 {
		rot := strings.TrimSpace(lines[3])
		if rot == "90" || rot == "-90" || rot == "270" {
			w, h = h, w
		}
	}

	return w + "x" + h
}
