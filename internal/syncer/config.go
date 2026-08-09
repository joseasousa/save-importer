package syncer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadConfig(path, defaultUserdata string) (Config, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultConfig(defaultUserdata), nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("configuração inválida: %w", err)
	}
	if c.Version != 1 {
		return Config{}, fmt.Errorf("versão de configuração não suportada: %d", c.Version)
	}
	if c.Userdata == "" {
		c.Userdata = defaultUserdata
	}
	return c, nil
}

func LoadIndex(path string) (Index, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Index{Version: 1, Files: map[string]IndexRecord{}}, nil
	}
	if err != nil {
		return Index{}, err
	}
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil || idx.Version != 1 {
		backup := path + ".corrupt"
		_ = os.Rename(path, backup)
		return Index{Version: 1, Files: map[string]IndexRecord{}}, nil
	}
	if idx.Files == nil {
		idx.Files = map[string]IndexRecord{}
	}
	return idx, nil
}

func SaveIndex(path string, idx Index) error { return writeJSONAtomic(path, idx) }

func SaveConfig(path string, cfg Config) error { return writeJSONAtomic(path, cfg) }

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
