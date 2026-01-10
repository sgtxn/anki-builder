package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var pathList = []string{
	"config.json",
	"$XDG_CONFIG_HOME/anki-builder/config.json",
}

type Config struct {
	LanguageSettings map[string]LanguageConfig `json:"languages"`
	GeminiAPIKey     string                    `json:"geminiApiKey"`
	GeminiModels     []string                  `json:"geminiModels"`
	AnkiConnectURL   string                    `json:"ankiConnectUrl"`
}

type LanguageConfig struct {
	DeckName   string `json:"deckName"`
	ModelName  string `json:"modelName"`
	PromptFile string `json:"promptFile"`
}

func Load() (*Config, error) {
	var cfgBytes []byte

	for _, path := range pathList {
		var err error
		path = os.ExpandEnv(path)
		cfgBytes, err = os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("found but failed to read config file %s: %w", path, err)
		}

		break
	}

	if cfgBytes == nil {
		return nil, fmt.Errorf("no config file found in paths: \n%s", strings.Join(pathList, "\n"))
	}

	var cfg Config
	err := json.Unmarshal(cfgBytes, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %w", err)
	}

	if err = validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if len(cfg.LanguageSettings) == 0 {
		return errors.New("at least one language must be specified in config file")
	}

	for lang, langCfg := range cfg.LanguageSettings {
		if langCfg.DeckName == "" {
			return fmt.Errorf("deck name is required for language %s", lang)
		}
		if langCfg.ModelName == "" {
			return fmt.Errorf("model name is required for language %s", lang)
		}
		if langCfg.PromptFile == "" {
			return fmt.Errorf("prompt file is required for language %s", lang)
		}
	}

	if cfg.AnkiConnectURL == "" {
		return errors.New("anki connect url config value is required")
	}

	if cfg.GeminiAPIKey == "" {
		return errors.New("gemini api key config value is required")
	}

	if len(cfg.GeminiModels) == 0 {
		return errors.New("at least one gemini model must be specified in config file")
	}

	return nil
}
