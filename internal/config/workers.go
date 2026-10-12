package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	delegatedomain "github.com/VladMallory/quickFFMPEG/internal/delegate/domain"
)

func WorkersFile(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if _, err := os.Stat("workers.json"); err == nil {
		abs, err := filepath.Abs("workers.json")
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "quickffmpeg", "workers.json"), nil
}

func LoadWorkerAt(path, name string) (delegatedomain.WorkerProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return delegatedomain.WorkerProfile{}, fmt.Errorf(
			"не читается %s (укажи --workers или положи workers.json рядом): %v", path, err,
		)
	}
	var all map[string]delegatedomain.WorkerProfile
	if err := json.Unmarshal(data, &all); err != nil {
		return delegatedomain.WorkerProfile{}, fmt.Errorf("битый %s: %v", path, err)
	}
	w, ok := all[name]
	if !ok {
		return delegatedomain.WorkerProfile{}, fmt.Errorf("профиль %q не найден в %s", name, path)
	}
	if w.Host == "" || w.User == "" {
		return delegatedomain.WorkerProfile{}, fmt.Errorf("в профиле %q нужны host и user", name)
	}
	if w.Port == 0 {
		w.Port = 22
	}
	if w.Dir == "" {
		w.Dir = "~/qff"
	}
	if w.Power <= 0 {
		w.Power = 1
	}
	if w.Slots <= 0 {
		w.Slots = 1
	}
	w.Name = name
	return w, nil
}
