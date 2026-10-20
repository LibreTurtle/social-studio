package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"google.golang.org/genai"
)

var ErrProvidersUnavailable = errors.New("all configured AI providers are unavailable")

type provider interface {
	Name() string
	Generate(context.Context, Prompt) (string, error)
}

type Generator struct {
	providers []provider
	timeout   time.Duration
	logger    *slog.Logger
}

func NewGenerator(settings Settings, logger *slog.Logger) (*Generator, error) {
	if settings.RequestTimeout <= 0 {
		settings.RequestTimeout = 20 * time.Second
	}
	if settings.MaxOutputTokens <= 0 {
		settings.MaxOutputTokens = 2048
	}
	if logger == nil {
		logger = slog.Default()
	}
	transport := settings.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	generator := &Generator{timeout: settings.RequestTimeout, logger: logger}
	for _, configured := range settings.Providers {
		if strings.TrimSpace(configured.APIKey) == "" {
			continue
		}
		base, err := url.Parse(configured.BaseURL)
		if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
			return nil, fmt.Errorf("invalid base URL for provider %s", configured.Name)
		}
		if strings.TrimSpace(configured.Model) == "" {
			return nil, fmt.Errorf("model is required for provider %s", configured.Name)
		}
		providerSettings := providerSettings{ProviderSettings: configured, client: client, maxOutputTokens: settings.MaxOutputTokens}
		switch configured.Name {
		case ProviderGroq:
			generator.providers = append(generator.providers, openAIProvider{providerSettings: providerSettings})
		case ProviderOpenRouter:
			openRouterClient := openrouter.New(
				openrouter.WithServerURL(strings.TrimRight(configured.BaseURL, "/")),
				openrouter.WithSecurity(configured.APIKey),
				openrouter.WithClient(client),
			)
			generator.providers = append(generator.providers, openRouterProvider{providerSettings: providerSettings, client: openRouterClient})
		case ProviderGemini:
			baseURL := strings.TrimSuffix(strings.TrimRight(configured.BaseURL, "/"), "/v1beta")
			geminiClient, err := genai.NewClient(context.Background(), &genai.ClientConfig{
				Backend:    genai.BackendGeminiAPI,
				APIKey:     configured.APIKey,
				HTTPClient: client,
				HTTPOptions: genai.HTTPOptions{
					BaseURL:    baseURL,
					APIVersion: "v1beta",
				},
			})
			if err != nil {
				return nil, fmt.Errorf("configure Gemini provider: %w", err)
			}
			generator.providers = append(generator.providers, geminiProvider{providerSettings: providerSettings, client: geminiClient})
		default:
			return nil, fmt.Errorf("unknown AI provider %q", configured.Name)
		}
	}
	return generator, nil
}

func (generator *Generator) Generate(ctx context.Context, prompt Prompt) (string, string, error) {
	if len(generator.providers) == 0 {
		return "", "", fmt.Errorf("%w: no configured providers", ErrProvidersUnavailable)
	}
	failedProviders := make([]string, 0, len(generator.providers))
	for _, configured := range generator.providers {
		providerContext, cancel := context.WithTimeout(ctx, generator.timeout)
		content, err := configured.Generate(providerContext, prompt)
		cancel()
		if err == nil && strings.TrimSpace(content) != "" {
			return strings.TrimSpace(content), configured.Name(), nil
		}
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		generator.logger.WarnContext(ctx, "AI provider attempt failed", "provider", configured.Name(), "error", safeError(err))
		failedProviders = append(failedProviders, configured.Name())
	}
	return "", "", fmt.Errorf("%w: %s", ErrProvidersUnavailable, strings.Join(failedProviders, ", "))
}

func safeError(err error) string {
	if err == nil {
		return "empty provider response"
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "provider request timed out"
	case errors.Is(err, context.Canceled):
		return "provider request was canceled"
	default:
		return "provider request failed"
	}
}
