package domain

import (
	"strconv"

	converterdomain "github.com/VladMallory/quickFFMPEG/internal/converter/domain"
)

type WorkerProfile struct {
	Name  string `json:"-"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	User  string `json:"user"`
	Dir   string `json:"dir"`
	Power int    `json:"power"`
	Slots int    `json:"slots"`
}

type WorkerLoad struct {
	One   string
	Cores int
}

func AssignSegments(sizes []int64, powers []int) []int {
	assign := make([]int, len(sizes))
	loads := make([]float64, len(powers))
	for _, idx := range orderDesc(sizes) {
		best := 0
		for w := 1; w < len(powers); w++ {
			if loads[w] < loads[best] {
				best = w
			}
		}
		assign[idx] = best
		power := powers[best]
		if power <= 0 {
			power = 1
		}
		loads[best] += float64(sizes[idx]) / float64(power)
	}
	return assign
}

func orderDesc(sizes []int64) []int {
	idx := make([]int, len(sizes))
	for i := range idx {
		idx[i] = i
	}
	for i := 1; i < len(idx); i++ {
		for j := i; j > 0 && sizes[idx[j]] > sizes[idx[j-1]]; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	return idx
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
