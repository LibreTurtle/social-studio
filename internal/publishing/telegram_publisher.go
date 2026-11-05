package publishing

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"encoding/json/v2"
)

const telegramAPIEndpoint = "https://api.telegram.org"

type TelegramPublisher struct {
	botToken     string
	chatID       string
	client       *http.Client
	endpointBase string
}

func NewTelegramPublisher(botToken, chatID string, client *http.Client) *TelegramPublisher {
	return &TelegramPublisher{botToken: strings.TrimSpace(botToken), chatID: strings.TrimSpace(chatID), client: safeProviderClient(client), endpointBase: telegramAPIEndpoint}
}

func (publisher *TelegramPublisher) Publish(ctx context.Context, command PublishCommand) (PublishResult, error) {
	if publisher.botToken == "" || publisher.chatID == "" || strings.TrimSpace(command.Content) == "" {
		return PublishResult{}, ErrPublisherUnavailable
	}
	endpoint := strings.TrimRight(publisher.endpointBase, "/") + "/bot" + url.PathEscape(publisher.botToken) + "/sendMessage"
	body, _, err := postJSON(ctx, publisher.client, endpoint, "", nil, struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{ChatID: publisher.chatID, Text: command.Content})
	if err != nil {
		return PublishResult{}, err
	}
	var response struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return PublishResult{}, ErrUncertain
	}
	if !response.OK {
		return PublishResult{}, ErrRejected
	}
	if response.Result.MessageID <= 0 {
		return PublishResult{}, ErrUncertain
	}
	return PublishResult{ExternalPostID: fmt.Sprint(response.Result.MessageID)}, nil
}
