package ai

import (
	"context"
	"errors"

	"google.golang.org/genai"
	"google.golang.org/genai/interactions/models/interactions"
	"google.golang.org/genai/interactions/models/operations"
)

type geminiProvider struct {
	providerSettings
	client *genai.Client
}

func (provider geminiProvider) Name() string { return ProviderGemini }

func (provider geminiProvider) Generate(ctx context.Context, prompt Prompt) (string, error) {
	store := false
	maxOutputTokens := provider.maxOutputTokens
	userPrompt, err := buildUserPrompt(prompt)
	if err != nil {
		return "", errors.New("could not encode provider prompt")
	}
	input := interactions.NewInteractionsInput(userPrompt)
	systemInstruction := systemPrompt
	response, err := provider.client.Interactions.Create(ctx, operations.CreateInteractionRequest{
		Body: operations.NewCreateInteractionRequestBody(interactions.CreateModelInteraction{
			Model:             interactions.Model(provider.Model),
			Input:             &input,
			SystemInstruction: &systemInstruction,
			Store:             &store,
			GenerationConfig: &interactions.GenerationConfig{
				MaxOutputTokens: &maxOutputTokens,
			},
		}),
	})
	if err != nil {
		return "", err
	}
	if response == nil || response.Interaction == nil || response.Interaction.OutputText == nil {
		return "", errors.New("Gemini returned no text output")
	}
	return *response.Interaction.OutputText, nil
}
