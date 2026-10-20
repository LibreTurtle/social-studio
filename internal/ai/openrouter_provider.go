package ai

import (
	"context"
	"errors"
	"strings"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

type openRouterProvider struct {
	providerSettings
	client *openrouter.OpenRouter
}

func (provider openRouterProvider) Name() string { return ProviderOpenRouter }

func (provider openRouterProvider) Generate(ctx context.Context, prompt Prompt) (string, error) {
	model := provider.Model
	maxTokens := int64(provider.maxOutputTokens)
	temperature := 0.3
	userPrompt, err := buildUserPrompt(prompt)
	if err != nil {
		return "", errors.New("could not encode provider prompt")
	}
	response, err := provider.client.Chat.Send(ctx, components.ChatRequest{
		Model: &model,
		Messages: []components.ChatMessages{
			components.CreateChatMessagesSystem(components.ChatSystemMessage{
				Content: components.CreateChatSystemMessageContentStr(systemPrompt),
			}),
			components.CreateChatMessagesUser(components.ChatUserMessage{
				Content: components.CreateChatUserMessageContentStr(userPrompt),
			}),
		},
		MaxTokens:   optionalnullable.From(&maxTokens),
		Temperature: optionalnullable.From(&temperature),
	}, nil)
	if err != nil {
		return "", err
	}
	if response == nil || response.ChatResult == nil || len(response.ChatResult.Choices) == 0 {
		return "", errors.New("OpenRouter returned no completion choices")
	}
	content, set := response.ChatResult.Choices[0].Message.Content.Get()
	if !set || content == nil {
		return "", errors.New("OpenRouter returned an empty completion")
	}
	if content.Str != nil {
		return *content.Str, nil
	}
	var text strings.Builder
	for _, part := range content.ArrayOfChatContentItems {
		if part.ChatContentText != nil {
			text.WriteString(part.ChatContentText.Text)
		}
	}
	if text.Len() == 0 {
		return "", errors.New("OpenRouter returned no text content")
	}
	return text.String(), nil
}
