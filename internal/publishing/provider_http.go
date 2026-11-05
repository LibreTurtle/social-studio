package publishing

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"time"
)

const (
	providerRequestTimeout = 20 * time.Second
	maximumProviderBody    = 64 << 10
)

var sharedProviderHTTPClient = newProviderHTTPClient()

func NewHTTPClient() *http.Client { return newProviderHTTPClient() }

func newProviderHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.MaxConnsPerHost = 20
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	return &http.Client{
		Transport: transport,
		Timeout:   providerRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func safeProviderClient(client *http.Client) *http.Client {
	if client == nil {
		return sharedProviderHTTPClient
	}
	configured := *client
	if configured.Timeout <= 0 || configured.Timeout > providerRequestTimeout {
		configured.Timeout = providerRequestTimeout
	}
	configured.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &configured
}

func postJSON(ctx context.Context, client *http.Client, endpoint, authorization string, headers http.Header, payload any) ([]byte, http.Header, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, ErrUncertain
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, nil, ErrUncertain
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, ErrUncertain
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumProviderBody+1))
	if err != nil || len(body) > maximumProviderBody {
		return nil, nil, ErrUncertain
	}
	if response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError {
		if response.StatusCode == http.StatusRequestTimeout {
			return nil, response.Header, ErrUncertain
		}
		return nil, response.Header, ErrRejected
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.Header, ErrUncertain
	}
	return body, response.Header, nil
}
