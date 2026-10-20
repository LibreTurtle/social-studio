package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxProviderResponseBytes = 1 << 20

type providerSettings struct {
	ProviderSettings
	client          *http.Client
	maxOutputTokens int
}

func (settings providerSettings) post(ctx context.Context, endpoint string, body []byte, apiKeyHeader, apiKeyPrefix string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("could not create provider request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(apiKeyHeader, apiKeyPrefix+settings.APIKey)
	response, err := settings.client.Do(request)
	if err != nil {
		return nil, errors.New("provider request failed")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxProviderResponseBytes+1))
	if err != nil || len(responseBody) > maxProviderResponseBytes {
		return nil, errors.New("provider response could not be read")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("provider returned HTTP %d", response.StatusCode)
	}
	return responseBody, nil
}
