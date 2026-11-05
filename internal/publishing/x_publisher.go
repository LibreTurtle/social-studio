package publishing

import (
	"context"
	"net/http"
	"strings"

	"encoding/json/v2"
)

const xPostsEndpoint = "https://api.x.com/2/tweets"

type XPublisher struct {
	accessToken string
	client      *http.Client
	endpoint    string
}

func NewXPublisher(accessToken string, client *http.Client) *XPublisher {
	return &XPublisher{accessToken: strings.TrimSpace(accessToken), client: safeProviderClient(client), endpoint: xPostsEndpoint}
}

func (publisher *XPublisher) Publish(ctx context.Context, command PublishCommand) (PublishResult, error) {
	if publisher.accessToken == "" || strings.TrimSpace(command.Content) == "" {
		return PublishResult{}, ErrPublisherUnavailable
	}
	body, _, err := postJSON(ctx, publisher.client, publisher.endpoint, "Bearer "+publisher.accessToken, nil, struct {
		Text string `json:"text"`
	}{Text: command.Content})
	if err != nil {
		return PublishResult{}, err
	}
	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || strings.TrimSpace(response.Data.ID) == "" {
		return PublishResult{}, ErrUncertain
	}
	return PublishResult{ExternalPostID: response.Data.ID}, nil
}
