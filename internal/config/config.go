package config

import (
	"flag"
	"log"

	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
)

func Load() domain.Config {
	cfg := domain.Config{}

	flag.StringVar(
		&cfg.Path, "path",
		".", "путь к директории с видеофайлами",
	)

	flag.StringVar(
		&cfg.Codec, "codec",
		"av1", "кодек av1 или h265",
	)

	flag.StringVar(
		&cfg.CRF, "crf",
		"28", "степень сжатия",
	)

	flag.IntVar(
		&cfg.Resize, "resize",
		0, "разрешение (720, 1080 и т.д.)",
	)

	flag.StringVar(
		&cfg.Preset, "preset",
		"", "пресет скорости/качества",
	)

	flag.StringVar(
		&cfg.AudioCodec, "audio",
		"opus", "аудиокодек aac или opus",
	)

	flag.IntVar(
		&cfg.MaxFps, "maxFps",
		24, "максимальный FPS (0 = без лимита)",
	)

	flag.BoolVar(
		&cfg.PerFps, "perFps",
		true, "переменная частота кадров (VFR) через scene detection",
	)

	flag.IntVar(
		&cfg.FilmGrain,
		"grain", 0,
		"уровень синтеза зерна для AV1 (от 0 до 50, 0 = выкл)",
	)

	flag.IntVar(
		&cfg.BitDepth, "bit",
		10, "битность видео: 8 или 10",
	)

	flag.StringVar(
		&cfg.Security, "security",
		"", "профиль воркера из workers.json: шифровать и кодировать удаленно",
	)

	flag.StringVar(
		&cfg.CryptIn, "crypt-in",
		"", "режим воркера: входной шифрованный файл",
	)

	flag.StringVar(
		&cfg.CryptOut, "crypt-out",
		"", "режим воркера: выходной шифрованный файл",
	)

	flag.Parse()

	if err := cfg.ApplyDefaults(); err != nil {
		log.Fatalln(err)
	}

	return cfg
}
