// Package config loads .cage/config.yaml with embedded defaults. A missing
// config file is not an error — defaults apply; a malformed one is.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// BackendCfg selects and parameterizes the model backend.
type BackendCfg struct {
	Type   string            `yaml:"type"`
	Params map[string]string `yaml:"params"`
}

// Config is the whole .cage/config.yaml surface.
type Config struct {
	Backend BackendCfg `yaml:"backend"`
	DOD     string     `yaml:"dod"`
	Jail    struct {
		Root string `yaml:"root"`
	} `yaml:"jail"`
	Budget struct {
		MaxTokens     int `yaml:"max_tokens"`
		PromptCeiling int `yaml:"prompt_ceiling"`
	} `yaml:"budget"`
	Memory struct {
		Enabled bool   `yaml:"enabled"`
		Path    string `yaml:"path"`
	} `yaml:"memory"`
	Quality struct {
		Disable []string `yaml:"disable"`
	} `yaml:"quality"`
	CooldownSeconds int `yaml:"cooldown_seconds"`
}

// Defaults returns the built-in configuration: local llama-server backend,
// jailed workspace under .cage/jail, context diet on, local memory off.
func Defaults() *Config {
	c := &Config{}
	c.Backend.Type = "llama_cpp"
	c.Backend.Params = map[string]string{
		"base_url": "http://127.0.0.1:8080",
		"model":    "local",
	}
	c.DOD = ""
	c.Jail.Root = filepath.Join(".cage", "jail")
	c.Budget.MaxTokens = 24000
	c.Budget.PromptCeiling = 8000
	c.Memory.Enabled = false
	c.Memory.Path = filepath.Join(".cage", "memories.jsonl")
	c.CooldownSeconds = 10
	return c
}

// Path returns the config file location for a project dir.
func Path(dir string) string {
	return filepath.Join(dir, ".cage", "config.yaml")
}

// Load reads .cage/config.yaml under dir. Missing file → defaults.
func Load(dir string) (*Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(Path(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", Path(dir), err)
	}
	if cfg.Jail.Root == "" {
		cfg.Jail.Root = filepath.Join(".cage", "jail")
	}
	if cfg.Budget.MaxTokens <= 0 {
		cfg.Budget.MaxTokens = 24000
	}
	if cfg.Budget.PromptCeiling <= 0 {
		cfg.Budget.PromptCeiling = 8000
	}
	return cfg, nil
}

// DisabledChecks turns the quality.disable list into a set.
func (c *Config) DisabledChecks() map[string]bool {
	out := map[string]bool{}
	for _, code := range c.Quality.Disable {
		out[code] = true
	}
	return out
}
