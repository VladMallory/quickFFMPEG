package domain

import (
	"reflect"
	"testing"

	converterdomain "github.com/VladMallory/quickFFMPEG/internal/converter/domain"
)

func TestRemoteArgs(t *testing.T) {
	cfg := converterdomain.Config{
		Codec:      "av1",
		CRF:        "40",
		Resize:     720,
		Preset:     "",
		AudioCodec: "aac",
		MaxFps:     24,
		PerFps:     false,
		FilmGrain:  6,
		BitDepth:   8,
	}
	got := RemoteArgs(cfg, "in/job000.enc", "out/job000.enc")
	want := []string{
		"--codec", "av1",
		"--crf", "40",
		"--resize", "720",
		"--preset", "",
		"--audio", "aac",
		"--maxFps", "24",
		"--perFps=false",
		"--grain", "6",
		"--bit", "8",
		"--crypt-in", "in/job000.enc",
		"--crypt-out", "out/job000.enc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRemoteEnv(t *testing.T) {
	dec := converterdomain.Crypt{Key: "k1", IV: "i1"}
	enc := converterdomain.Crypt{Key: "k2", IV: "i2"}
	got := RemoteEnv(dec, enc)
	want := map[string]string{
		"QFF_KEY":  "k1",
		"QFF_IV":   "i1",
		"QFF_OKEY": "k2",
		"QFF_OIV":  "i2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
