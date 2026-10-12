package sshadapter

import "testing"

func TestFFmpegMajor(t *testing.T) {
	cases := map[string]int{
		"ffmpeg version n9.0.2 Copyright (c) 2000-2026": 9,
		"ffmpeg version 5.1.9-0+deb12u1 Copyright":      5,
		"ffmpeg version 4.4.2-0ubuntu0.22.04.1":         4,
		"ffmpeg version 7.1.1":                          7,
		"":                                              0,
		"garbage":                                       0,
	}
	for in, want := range cases {
		if got := ffmpegMajor(in); got != want {
			t.Fatalf("ffmpegMajor(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseLoad(t *testing.T) {
	got, err := parseLoad("0.22 0.10 0.04 1/234 5678\n12\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.One != "0.22" || got.Cores != 12 {
		t.Fatalf("got %+v", got)
	}
	if _, err := parseLoad(""); err == nil {
		t.Fatal("жди ошибку на пустом выводе")
	}
}
