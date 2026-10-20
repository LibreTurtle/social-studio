package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LibreTurtle/social-studio/internal/ai"
)

type Config struct {
	Port, DatabaseURL string
	AI                ai.Settings
}

func LoadFrom(lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("environment lookup is required")
	}
	get := func(key, fallback string) string {
		value, ok := lookup(key)
		if !ok {
			return fallback
		}
		return strings.TrimSpace(value)
	}
	config := Config{Port: get("PORT", "8080"), DatabaseURL: get("DATABASE_URL", "")}
	if config.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if strings.HasPrefix(config.Port, ":") || strings.ContainsAny(config.Port, "/\r\n") {
		return Config{}, errors.New("PORT is invalid")
	}
	timeout, err := time.ParseDuration(get("AI_REQUEST_TIMEOUT", "20s"))
	if err != nil || timeout <= 0 {
		return Config{}, errors.New("AI_REQUEST_TIMEOUT must be a positive duration")
	}
	maxOutput, err := strconv.Atoi(get("AI_MAX_OUTPUT_TOKENS", "2048"))
	if err != nil || maxOutput <= 0 {
		return Config{}, errors.New("AI_MAX_OUTPUT_TOKENS must be positive")
	}
	config.AI.RequestTimeout, config.AI.MaxOutputTokens = timeout, maxOutput
	providers := []struct{ name, enabledKey, keyKey, modelKey, baseKey, baseURL, model string }{
		{ai.ProviderGroq, "GROQ_ENABLED", "GROQ_API_KEY", "GROQ_MODEL", "GROQ_BASE_URL", "https://api.groq.com/openai/v1", "openai/gpt-oss-20b"},
		{ai.ProviderGemini, "GEMINI_ENABLED", "GEMINI_API_KEY", "GEMINI_MODEL", "GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta", "gemini-3.5-flash-lite"},
		{ai.ProviderOpenRouter, "OPENROUTER_ENABLED", "OPENROUTER_API_KEY", "OPENROUTER_MODEL", "OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1", "openrouter/free"},
	}
	for _, provider := range providers {
		enabled, parseErr := strconv.ParseBool(get(provider.enabledKey, "false"))
		if parseErr != nil {
			return Config{}, fmt.Errorf("%s must be true or false", provider.enabledKey)
		}
		if !enabled {
			continue
		}
		key := get(provider.keyKey, "")
		if key == "" {
			return Config{}, fmt.Errorf("%s is required when %s is true", provider.keyKey, provider.enabledKey)
		}
		config.AI.Providers = append(config.AI.Providers, ai.ProviderSettings{Name: provider.name, BaseURL: get(provider.baseKey, provider.baseURL), APIKey: key, Model: get(provider.modelKey, provider.model)})
	}
	return config, nil
}

func Load() (Config, error) {
	return LoadFrom(func(key string) (string, bool) { value, ok := os.LookupEnv(key); return value, ok })
}
