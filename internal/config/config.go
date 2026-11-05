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
	Publishing        PublishingSettings
}

type PublishingSettings struct {
	X        XPublishingSettings
	LinkedIn LinkedInPublishingSettings
	Telegram TelegramPublishingSettings
}

type XPublishingSettings struct {
	Enabled     bool
	AccessToken string
}

type LinkedInPublishingSettings struct {
	Enabled     bool
	AccessToken string
	AuthorURN   string
	APIVersion  string
}

type TelegramPublishingSettings struct {
	Enabled  bool
	BotToken string
	ChatID   string
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
	config.Publishing.X.Enabled, err = parseFlag(get, "X_PUBLISH_ENABLED")
	if err != nil {
		return Config{}, err
	}
	config.Publishing.X.AccessToken = get("X_ACCESS_TOKEN", "")
	if config.Publishing.X.Enabled && config.Publishing.X.AccessToken == "" {
		return Config{}, errors.New("X_ACCESS_TOKEN is required when X_PUBLISH_ENABLED is true")
	}
	config.Publishing.LinkedIn.Enabled, err = parseFlag(get, "LINKEDIN_PUBLISH_ENABLED")
	if err != nil {
		return Config{}, err
	}
	config.Publishing.LinkedIn.AccessToken = get("LINKEDIN_ACCESS_TOKEN", "")
	config.Publishing.LinkedIn.AuthorURN = get("LINKEDIN_AUTHOR_URN", "")
	config.Publishing.LinkedIn.APIVersion = get("LINKEDIN_API_VERSION", "202610")
	if config.Publishing.LinkedIn.Enabled {
		if config.Publishing.LinkedIn.AccessToken == "" || config.Publishing.LinkedIn.AuthorURN == "" {
			return Config{}, errors.New("LINKEDIN_ACCESS_TOKEN and LINKEDIN_AUTHOR_URN are required when LINKEDIN_PUBLISH_ENABLED is true")
		}
		version := config.Publishing.LinkedIn.APIVersion
		if len(version) != 6 || !strings.HasPrefix(version, "20") || !digits(version[2:]) {
			return Config{}, errors.New("LINKEDIN_API_VERSION must use YYYYMM format")
		}
		if !strings.HasPrefix(config.Publishing.LinkedIn.AuthorURN, "urn:li:person:") && !strings.HasPrefix(config.Publishing.LinkedIn.AuthorURN, "urn:li:organization:") {
			return Config{}, errors.New("LINKEDIN_AUTHOR_URN must identify a person or organization")
		}
	}
	config.Publishing.Telegram.Enabled, err = parseFlag(get, "TELEGRAM_PUBLISH_ENABLED")
	if err != nil {
		return Config{}, err
	}
	config.Publishing.Telegram.BotToken = get("TELEGRAM_BOT_TOKEN", "")
	config.Publishing.Telegram.ChatID = get("TELEGRAM_CHAT_ID", "")
	if config.Publishing.Telegram.Enabled && (config.Publishing.Telegram.BotToken == "" || config.Publishing.Telegram.ChatID == "") {
		return Config{}, errors.New("TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID are required when TELEGRAM_PUBLISH_ENABLED is true")
	}
	return config, nil
}

func Load() (Config, error) {
	return LoadFrom(func(key string) (string, bool) { value, ok := os.LookupEnv(key); return value, ok })
}

func parseFlag(get func(string, string) string, key string) (bool, error) {
	value, err := strconv.ParseBool(get(key, "false"))
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func digits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
