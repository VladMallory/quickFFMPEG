package convertersvc

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
)

type Encoder interface {
	Encode(ctx context.Context, cfg domain.Config, inputPath, outputPath string) error
}

type Prober interface {
	Resolution(ctx context.Context, path string) string
}

type FileStore interface {
	MkdirAll(path string) error
	ListFiles(dir string) ([]string, error)
	IsDir(path string) bool
	SizeMB(path string) (float64, error)
	SaveSettings(dir string, cfg domain.Config) error
}

type ConverterService struct {
	encoder Encoder
	prober  Prober
	store   FileStore
}

func NewConverterService(
	encoder Encoder,
	prober Prober,
	store FileStore,
) ConverterService {
	return ConverterService{
		encoder: encoder,
		prober:  prober,
		store:   store,
	}
}

func (s ConverterService) ConvertAll(
	ctx context.Context,
	cfg domain.Config,
	onEncoded func(outputPath string),
) ([]domain.FileStat, error) {

	compressedDir := filepath.Join(cfg.Path, "compressed")
	if err := s.store.MkdirAll(compressedDir); err != nil {
		return nil, fmt.Errorf("Не удалось создать папку для сжатых файлов: %v", err)
	}

	if err := s.store.SaveSettings(compressedDir, cfg); err != nil {
		return nil, err
	}

	names, err := s.store.ListFiles(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("Ошибка чтения директории: %v", err)
	}

	var stats []domain.FileStat

	for _, name := range names {
		inputPath := filepath.Join(cfg.Path, name)

		if s.store.IsDir(inputPath) {
			continue
		}

		ext := strings.ToLower(filepath.Ext(inputPath))
		if !domain.ValidExtensions[ext] {
			continue
		}

		resBefore := s.prober.Resolution(ctx, inputPath)

		sizeBefore, err := s.store.SizeMB(inputPath)
		if err != nil {
			return nil, fmt.Errorf("не удалось получить статистику файла %v", err)
		}

		baseName := strings.TrimSuffix(name, filepath.Ext(name))
		outputPath := filepath.Join(compressedDir, baseName+".mp4")

		if err := s.encoder.Encode(ctx, cfg, inputPath, outputPath); err != nil {
			return nil, fmt.Errorf("err: %v", err)
		}

		if onEncoded != nil {
			onEncoded(outputPath)
		}

		resAfter := s.prober.Resolution(ctx, outputPath)

		sizeAfter, err := s.store.SizeMB(outputPath)
		if err != nil {
			return nil, fmt.Errorf("не удалось получить размер сжатого файла %v", err)
		}

		stats = append(stats, domain.FileStat{
			Name:             name,
			OutputPath:       outputPath,
			SizeBefore:       sizeBefore,
			SizeAfter:        sizeAfter,
			ResolutionBefore: resBefore,
			ResolutionAfter:  resAfter,
		})
	}

	return stats, nil
}
