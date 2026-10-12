package domain

import (
	"strconv"

	converterdomain "github.com/VladMallory/quickFFMPEG/internal/converter/domain"
)

type WorkerProfile struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Dir  string `json:"dir"`
}

func RemoteEnv(dec, enc converterdomain.Crypt) map[string]string {
	return map[string]string{
		converterdomain.EnvKey:  dec.Key,
		converterdomain.EnvIV:   dec.IV,
		converterdomain.EnvOKey: enc.Key,
		converterdomain.EnvOIV:  enc.IV,
	}
}

func RemoteArgs(cfg converterdomain.Config, inPath, outPath string) []string {
	return []string{
		"--codec", cfg.Codec,
		"--crf", cfg.CRF,
		"--resize", strconv.Itoa(cfg.Resize),
		"--preset", cfg.Preset,
		"--audio", cfg.AudioCodec,
		"--maxFps", strconv.Itoa(cfg.MaxFps),
		"--perFps=" + strconv.FormatBool(cfg.PerFps),
		"--grain", strconv.Itoa(cfg.FilmGrain),
		"--bit", strconv.Itoa(cfg.BitDepth),
		"--crypt-in", inPath,
		"--crypt-out", outPath,
	}
}
