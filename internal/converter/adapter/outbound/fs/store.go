package fs

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/VladMallory/quickFFMPEG/internal/converter/domain"
)

type FileStoreAdapter struct{}

func New() *FileStoreAdapter {
	return &FileStoreAdapter{}
}

func (a *FileStoreAdapter) MkdirAll(path string) error {
	return os.MkdirAll(path, os.ModePerm)
}

func (a *FileStoreAdapter) ListFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	return names, nil
}

func (a *FileStoreAdapter) IsDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fi.IsDir()
}

// VideoDirs возвращает root и все вложенные директории, в которых есть
// видеофайлы. Папки compressed пропускаются, чтобы не сжимать результат
// повторно. Символические ссылки не разыменовываются.
func (a *FileStoreAdapter) VideoDirs(root string) ([]string, error) {
	var dirs []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && strings.EqualFold(entry.Name(), "compressed") {
			return filepath.SkipDir
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if domain.ValidExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
				dirs = append(dirs, path)
				break
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return dirs, nil
}

func (a *FileStoreAdapter) SizeMB(path string) (float64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	return float64(fi.Size() / 1048576), nil
}

func (a *FileStoreAdapter) SaveSettings(dir string, cfg domain.Config) error {
	jsonBytes, err := json.MarshalIndent(cfg, "", " ")
	if err != nil {
		return nil
	}

	settingsPath := filepath.Join(dir, "settings.txt")
	if err := os.WriteFile(settingsPath, jsonBytes, 0o644); err != nil {
		log.Printf("Ошибка сохранения настроек: %v", err)
	} else {
		fmt.Println("Настройки сохранены в:", settingsPath)
	}

	return nil
}
