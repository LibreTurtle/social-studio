package ai

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strings"
)

type openAIProvider struct{ providerSettings }

func (provider openAIProvider) Name() string { return provider.ProviderSettings.Name }

func (provider openAIProvider) Generate(ctx context.Context, prompt Prompt) (string, error) {
	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	userPrompt, err := buildUserPrompt(prompt)
	if err != nil {
		return "", errors.New("could not encode provider prompt")
	}
	body, err := json.Marshal(struct {
		Model       string        `json:"model"`
		Messages    []chatMessage `json:"messages"`
		MaxTokens   int           `json:"max_tokens"`
		Temperature float64       `json:"temperature"`
	}{Model: provider.Model, Messages: []chatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}}, MaxTokens: provider.maxOutputTokens, Temperature: 0.3})
	if err != nil {
		return "", errors.New("could not encode provider request")
	}
	response, err := provider.providerSettings.post(ctx, endpoint, body, "Authorization", "Bearer ")
	if err != nil {
		return "", err
	}
	var result struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(response, &result); err != nil || len(result.Choices) == 0 {
		return "", errors.New("provider returned an invalid response")
	}
	return result.Choices[0].Message.Content, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
