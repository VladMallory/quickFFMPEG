package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Допустимые расширения файлов
var validExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".mov":  true,
	".avi":  true,
	".webm": true,
}

// fileStat хранит информацию о файле.
type fileStat struct {
	name             string
	sizeBefore       float64
	sizeAfter        float64
	resolutionBefore string
	resolutionAfter  string
}

// config описывает флаги для приложения.
type config struct {
	Codec           string `json:"codec"`
	Path            string `json:"path"`
	CRF             string `json:"crf"`
	Resize          int    `json:"resize"`
	Preset          string `json:"preset"`
	AudioCodec      string `json:"audio_codec"`
	AudioCodecValue string `json:"audio_codecValue"`
	AudioBitrate    string `json:"audio_bitrate"`
	MaxFps          int    `json:"maxFps"`
	PerFps          bool   `json:"perFps"`
	FilmGrain       int    `json:"film_grain"`
	BitDepth        int    `json:"bit_depth"`
}

func NewConfig() *config {
	cfg := &config{}

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

	flag.Parse()

	if cfg.Codec != "av1" && cfg.Codec != "265" {
		log.Fatalln("неизвестный кодек")
	}

	if cfg.AudioCodec != "aac" && cfg.AudioCodec != "opus" {
		log.Fatalln("неизвестный аудиокодек")
	}

	switch cfg.AudioCodec {
	case "opus":
		cfg.AudioCodecValue = "libopus"
		cfg.AudioBitrate = "96k"
	case "aac":
		cfg.AudioCodecValue = "aac"
		cfg.AudioBitrate = "128k"
	}

	if cfg.Preset == "" {
		if cfg.Codec == "265" {
			cfg.Preset = "slow"
		} else {
			cfg.Preset = "1"
		}
	}

	return cfg
}

func main() {
	buildTime := "unknown"
	if exe, err := os.Executable(); err == nil {
		if fi, err := os.Stat(exe); err == nil && !fi.ModTime().IsZero() {
			buildTime = fi.ModTime().Format(time.RFC3339)
		}
	}
	fmt.Println("Build time:", buildTime)

	cfg := NewConfig()

	// Создаем папку для сжатых файлов
	compressedDir := filepath.Join(cfg.Path, "compressed")
	if err := os.MkdirAll(compressedDir, os.ModePerm); err != nil {
		log.Fatalf("Не удалось создать папку для сжатых файлов: %v", err)
	}

	// Читаем файлы в директории
	files, err := os.ReadDir(cfg.Path)
	if err != nil {
		log.Fatalf("Ошибка чтения директории: %v", err)
	}

	// для хранения информации о файлах
	var fileStats []fileStat

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		inputPath := filepath.Join(cfg.Path, file.Name())
		ext := strings.ToLower(filepath.Ext(inputPath))

		// Проверяем, подходит ли нам формат
		if !validExtensions[ext] {
			continue
		}

		resBefore := getResolution(inputPath)

		// размер файла до обработки
		fileInfoBefore, err := os.Stat(inputPath)
		if err != nil {
			log.Fatal("не удалось получить статистику файла", err)
		}

		sizeBeforeMB := float64(fileInfoBefore.Size() / 1048576)

		// Формируем имя выходного файла (меняем расширение на .mp4)
		baseName := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		outputPath := filepath.Join(compressedDir, baseName+".mp4")

		args := []string{
			"-i", inputPath,
			"-crf", cfg.CRF,
		}

		// 1. Кодеки
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

		// 2. Видео-фильтры (накапливаем их)
		var filters []string
		if cfg.MaxFps > 0 && !cfg.PerFps {
			args = append(args, "-r", strconv.Itoa(cfg.MaxFps))
		}
		if cfg.PerFps {
			args = append(args, "-vsync", "vfr")
			if cfg.MaxFps > 0 {
				filters = append(filters, fmt.Sprintf("fps=%d", cfg.MaxFps))
			}
			filters = append(filters, "mpdecimate")
		}
		if cfg.Resize > 0 {
			box := cfg.Resize * 16 / 9
			filters = append(filters, fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", box, box))
		}
		if len(filters) > 0 {
			args = append(args, "-vf", strings.Join(filters, ","))
		}

		// 3. Пиксельный формат (10-бит для AV1 и H265)
		pixFmt := "yuv420p"
		if cfg.BitDepth == 10 {
			pixFmt = "yuv420p10le"
		}
		args = append(args, "-pix_fmt", pixFmt, "-preset", cfg.Preset)

		// 4. Аудио (один раз!)
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
				"-af", "silenceremove=start_periods=0:stop_periods=0:start_threshold=-60dB",
			)
		}

		// 5. Выходной файл
		args = append(args, outputPath)

		cmd := exec.Command("ffmpeg", args...)

		// Перенаправляем лог FFmpeg в стандартный вывод нашей программы,
		// чтобы видеть прогресс кодирования прямо в консоли
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Fatal("err: ", err)
		} else {
			fmt.Printf("✅ Файл успешно сохранен в: %s\n", outputPath)

			resAfter := getResolution(outputPath)

			fileInfoAfter, err := os.Stat(outputPath)
			if err != nil {
				log.Fatal("не удалось получить размер сжатого файла")
				continue
			}

			sizeAfterMB := float64(fileInfoAfter.Size() / 1048576)

			fileStats = append(fileStats, fileStat{
				name:             file.Name(),
				sizeBefore:       sizeBeforeMB,
				sizeAfter:        sizeAfterMB,
				resolutionBefore: resBefore,
				resolutionAfter:  resAfter,
			})
		}
	}

	fmt.Println("\n🎉 Все файлы обработаны!")
	if len(fileStats) > 0 {
		fmt.Println("Статистика сжатия")
		for _, stat := range fileStats {
			fmt.Printf(
				"- %s, было: %.1fmb (%s), стало: %.1fmb (%s)\n",
				stat.name, stat.sizeBefore, stat.resolutionBefore,
				stat.sizeAfter, stat.resolutionAfter,
			)
		}

		jsonByte, err := json.MarshalIndent(cfg, "", " ")
		if err != nil {
			return
		}

		fmt.Println("Настройки: \n", string(jsonByte))

		// fmt.Printf("Настройки: %+v", cfg)

		// fmt.Printf(
		// 	"Настройки: \ncfg: %s\npreset: %s\nresize: %d\ncodec: %s\nmaxFps: %d\nperFps: %t",
		// 	cfg.crf, cfg.preset, cfg.resize, cfg.codec, cfg.maxFps, cfg.perFps,
		// )
	}
}

func getResolution(path string) string {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "frame=width,height:frame_side_data=side_data_type,rotation",
		"-read_intervals", "%+#1",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		return "N/A"
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "N/A"
	}

	w, h := lines[0], lines[1]

	// rotation в 4-й строке (после width, height, side_data_type)
	if len(lines) >= 4 {
		rot := strings.TrimSpace(lines[3])
		if rot == "90" || rot == "-90" || rot == "270" {
			w, h = h, w
		}
	}

	return w + "x" + h
}
