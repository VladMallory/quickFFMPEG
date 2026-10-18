package domain

import (
	"fmt"
	"strconv"
	"strings"
)

func BuildArgs(cfg Config, inputPath, outputPath string, dec, enc *Crypt) []string {
	args := []string{}

	inURL := inputPath
	if dec != nil {
		args = append(args, "-decryption_key", dec.Key, "-decryption_iv", dec.IV)
		inURL = "crypto:file://" + inputPath
	}

	args = append(args, "-i", inURL, "-crf", cfg.CRF)

	switch cfg.Codec {
	case "265":
		args = append(args, "-c:v", "libx265", "-tag:v", "hvc1")
	case "av1":
		args = append(args, "-c:v", "libsvtav1", "-g", "240")
		svtParams := "tune=0"
		if cfg.FilmGrain > 0 {
			svtParams += fmt.Sprintf(":film-grain=%d", cfg.FilmGrain)
		}
		args = append(args, "-svtav1-params", svtParams)
	}

	var filters []string
	if cfg.MaxFps > 0 && !cfg.PerFps {
		args = append(args, "-r", strconv.Itoa(cfg.MaxFps))
	}
	if cfg.PerFps {
		args = append(args, "-fps_mode", "vfr")
		if cfg.MaxFps > 0 {
			filters = append(filters, fmt.Sprintf("fps=%d", cfg.MaxFps))
		}
		filters = append(filters, "mpdecimate")
	}
	if cfg.Resize > 0 {
		box := cfg.Resize * 16 / 9
		filters = append(
			filters,
			fmt.Sprintf(
				"scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2",
				box, box,
			),
		)
	}
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}

	pixFmt := "yuv420p"
	if cfg.BitDepth == 10 {
		pixFmt = "yuv420p10le"
	}
	args = append(args, "-pix_fmt", pixFmt, "-preset", cfg.Preset)

	args = append(
		args,
		"-c:a", cfg.AudioCodecValue,
		"-b:a", cfg.AudioBitrate,
	)
	if cfg.AudioCodecValue == "libopus" {
		args = append(
			args,
			"-vbr", "on",
			"-compression_level", "10",
		)
	}

	outURL := outputPath
	if enc != nil {
		args = append(args,
			"-movflags", "frag_keyframe+empty_moov",
			"-f", "mp4",
			"-encryption_key", enc.Key,
			"-encryption_iv", enc.IV,
		)
		outURL = "crypto:file://" + outputPath
	}
	args = append(args, outURL)

	return args
}
