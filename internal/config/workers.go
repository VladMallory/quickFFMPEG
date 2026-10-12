package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	delegatedomain "github.com/VladMallory/quickFFMPEG/internal/delegate/domain"
)

func LoadWorker(name string) (delegatedomain.WorkerProfile, error) {
	path, err := workerFile()
	if err != nil {
		return delegatedomain.WorkerProfile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return delegatedomain.WorkerProfile{}, fmt.Errorf("не читается %s: %v", path, err)
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
	return w, nil
}

func workerFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "quickffmpeg", "workers.json"), nil
}
