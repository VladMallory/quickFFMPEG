package domain

import (
	"reflect"
	"testing"
)

func testConfig() Config {
	c := Config{
		Codec:           "265",
		Path:            ".",
		CRF:             "30",
		Resize:          0,
		Preset:          "slow",
		AudioCodec:      "opus",
		AudioCodecValue: "libopus",
		AudioBitrate:    "96k",
		MaxFps:          0,
		PerFps:          true,
		FilmGrain:       0,
		BitDepth:        10,
	}
	return c
}

func TestBuildArgsPlain(t *testing.T) {
	got := BuildArgs(testConfig(), "in/a.mp4", "compressed/a.mp4", nil, nil)
	want := []string{
		"-i", "in/a.mp4",
		"-crf", "30",
		"-c:v", "libx265", "-tag:v", "hvc1",
		"-fps_mode", "vfr", "-vf", "mpdecimate",
		"-pix_fmt", "yuv420p10le", "-preset", "slow",
		"-c:a", "libopus", "-b:a", "96k",
		"-vbr", "on", "-compression_level", "10",
		"compressed/a.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildArgsCrypto(t *testing.T) {
	dec := &Crypt{Key: "aa", IV: "bb"}
	enc := &Crypt{Key: "cc", IV: "dd"}
	got := BuildArgs(testConfig(), "in/job.enc", "out/job.enc", dec, enc)
	want := []string{
		"-decryption_key", "aa", "-decryption_iv", "bb",
		"-i", "crypto:file://in/job.enc",
		"-crf", "30",
		"-c:v", "libx265", "-tag:v", "hvc1",
		"-fps_mode", "vfr", "-vf", "mpdecimate",
		"-pix_fmt", "yuv420p10le", "-preset", "slow",
		"-c:a", "libopus", "-b:a", "96k",
		"-vbr", "on", "-compression_level", "10",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4",
		"-encryption_key", "cc", "-encryption_iv", "dd",
		"crypto:file://out/job.enc",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildArgsResize(t *testing.T) {
	cfg := testConfig()
	cfg.Resize = 1080
	got := BuildArgs(cfg, "in/a.mp4", "compressed/a.mp4", nil, nil)
	want := []string{
		"-i", "in/a.mp4",
		"-crf", "30",
		"-c:v", "libx265", "-tag:v", "hvc1",
		"-fps_mode", "vfr",
		"-vf", "mpdecimate,scale='min(1920,iw)':'min(1920,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2",
		"-pix_fmt", "yuv420p10le", "-preset", "slow",
		"-c:a", "libopus", "-b:a", "96k",
		"-vbr", "on", "-compression_level", "10",
		"compressed/a.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
