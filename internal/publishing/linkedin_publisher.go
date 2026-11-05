package publishing

import (
	"context"
	"net/http"
	"strings"
)

const linkedInPostsEndpoint = "https://api.linkedin.com/rest/posts"

type LinkedInPublisher struct {
	accessToken string
	authorURN   string
	apiVersion  string
	client      *http.Client
	endpoint    string
}

func NewLinkedInPublisher(accessToken, authorURN, apiVersion string, client *http.Client) *LinkedInPublisher {
	return &LinkedInPublisher{
		accessToken: strings.TrimSpace(accessToken), authorURN: strings.TrimSpace(authorURN),
		apiVersion: strings.TrimSpace(apiVersion), client: safeProviderClient(client), endpoint: linkedInPostsEndpoint,
	}
}

func (publisher *LinkedInPublisher) Publish(ctx context.Context, command PublishCommand) (PublishResult, error) {
	if publisher.accessToken == "" || publisher.authorURN == "" || publisher.apiVersion == "" || strings.TrimSpace(command.Content) == "" {
		return PublishResult{}, ErrPublisherUnavailable
	}
	headers := make(http.Header)
	headers.Set("Linkedin-Version", publisher.apiVersion)
	headers.Set("X-Restli-Protocol-Version", "2.0.0")
	payload := struct {
		Author          string               `json:"author"`
		Commentary      string               `json:"commentary"`
		Visibility      string               `json:"visibility"`
		Distribution    linkedInDistribution `json:"distribution"`
		LifecycleState  string               `json:"lifecycleState"`
		ReshareDisabled bool                 `json:"isReshareDisabledByAuthor"`
	}{
		Author: publisher.authorURN, Commentary: command.Content, Visibility: "PUBLIC",
		Distribution:   linkedInDistribution{FeedDistribution: "MAIN_FEED", TargetEntities: []string{}, ThirdPartyDistributionChannels: []string{}},
		LifecycleState: "PUBLISHED",
	}
	_, responseHeaders, err := postJSON(ctx, publisher.client, publisher.endpoint, "Bearer "+publisher.accessToken, headers, payload)
	if err != nil {
		return PublishResult{}, err
	}
	postID := strings.TrimSpace(responseHeaders.Get("x-restli-id"))
	if postID == "" {
		return PublishResult{}, ErrUncertain
	}
	return PublishResult{ExternalPostID: postID}, nil
}

type linkedInDistribution struct {
	FeedDistribution               string   `json:"feedDistribution"`
	TargetEntities                 []string `json:"targetEntities"`
	ThirdPartyDistributionChannels []string `json:"thirdPartyDistributionChannels"`
}
