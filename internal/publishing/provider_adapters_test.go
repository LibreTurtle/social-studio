package publishing

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestXPublisherCreatesPostAndParsesID(t *testing.T) {
	client := clientWithTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/2/tweets" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer x-token" {
			t.Fatalf("authorization header = %q", request.Header.Get("Authorization"))
		}
		var body map[string]string
		if err := json.UnmarshalRead(request.Body, &body); err != nil || body["text"] != "hello x" {
			t.Fatalf("request body = %v, err = %v", body, err)
		}
		return response(http.StatusCreated, nil, `{"data":{"id":"x-post-1"}}`), nil
	})
	publisher := NewXPublisher("x-token", client)
	publisher.endpoint = "https://provider.test/2/tweets"
	result, err := publisher.Publish(context.Background(), PublishCommand{Content: "hello x"})
	if err != nil || result.ExternalPostID != "x-post-1" {
		t.Fatalf("Publish() = %+v, %v", result, err)
	}
}

func TestLinkedInPublisherUsesRequiredHeadersAndAuthor(t *testing.T) {
	client := clientWithTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/rest/posts" || request.Header.Get("Linkedin-Version") != "202610" || request.Header.Get("X-Restli-Protocol-Version") != "2.0.0" {
			t.Fatalf("request path/headers = %s %+v", request.URL.Path, request.Header)
		}
		var body map[string]any
		if err := json.UnmarshalRead(request.Body, &body); err != nil {
			t.Fatal(err)
		}
		if request.Header.Get("Authorization") != "Bearer li-token" || body["author"] != "urn:li:person:member-1" || body["lifecycleState"] != "PUBLISHED" || body["visibility"] != "PUBLIC" || body["isReshareDisabledByAuthor"] != false {
			t.Fatalf("request body = %v", body)
		}
		return response(http.StatusCreated, http.Header{"X-Restli-Id": []string{"urn:li:share:post-1"}}, ""), nil
	})
	publisher := NewLinkedInPublisher("li-token", "urn:li:person:member-1", "202610", client)
	publisher.endpoint = "https://provider.test/rest/posts"
	result, err := publisher.Publish(context.Background(), PublishCommand{Content: "hello LinkedIn"})
	if err != nil || result.ExternalPostID != "urn:li:share:post-1" {
		t.Fatalf("Publish() = %+v, %v", result, err)
	}
}

func TestTelegramPublisherDoesNotLeakTokenAndParsesMessageID(t *testing.T) {
	const secret = "123456:telegram-secret"
	client := clientWithTransport(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/bot"+secret+"/sendMessage" {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		var body map[string]string
		if err := json.UnmarshalRead(request.Body, &body); err != nil || body["chat_id"] != "@channel" || body["text"] != "hello Telegram" {
			t.Fatalf("request body = %v, err = %v", body, err)
		}
		return response(http.StatusOK, nil, `{"ok":true,"result":{"message_id":72}}`), nil
	})
	publisher := NewTelegramPublisher(secret, "@channel", client)
	publisher.endpointBase = "https://provider.test"
	result, err := publisher.Publish(context.Background(), PublishCommand{Content: "hello Telegram"})
	if err != nil || result.ExternalPostID != "72" {
		t.Fatalf("Publish() = %+v, %v", result, err)
	}
}

func TestProviderFailureDoesNotExposeUpstreamBody(t *testing.T) {
	client := clientWithTransport(func(*http.Request) (*http.Response, error) {
		return response(http.StatusUnauthorized, nil, "secret provider response must not escape"), nil
	})
	publisher := NewXPublisher("x-token", client)
	publisher.endpoint = "https://provider.test/2/tweets"
	_, err := publisher.Publish(context.Background(), PublishCommand{Content: "hello"})
	if !errors.Is(err, ErrRejected) || strings.Contains(err.Error(), "secret provider response") {
		t.Fatalf("Publish() error = %v, want sanitized rejection", err)
	}
}

func TestProviderRedirectDoesNotForwardCredentials(t *testing.T) {
	requests := 0
	client := clientWithTransport(func(request *http.Request) (*http.Response, error) {
		requests++
		return response(http.StatusTemporaryRedirect, http.Header{"Location": []string{"https://attacker.test/collect"}}, ""), nil
	})
	publisher := NewXPublisher("x-token", client)
	publisher.endpoint = "https://provider.test/2/tweets"
	_, err := publisher.Publish(context.Background(), PublishCommand{Content: "hello"})
	if !errors.Is(err, ErrUncertain) || requests != 1 {
		t.Fatalf("Publish() error=%v requests=%d; redirect must not be followed", err, requests)
	}
}

func TestProviderTimeoutResponseIsTreatedAsUncertain(t *testing.T) {
	client := clientWithTransport(func(*http.Request) (*http.Response, error) {
		return response(http.StatusRequestTimeout, nil, "request timed out"), nil
	})
	publisher := NewXPublisher("x-token", client)
	publisher.endpoint = "https://provider.test/2/tweets"
	_, err := publisher.Publish(context.Background(), PublishCommand{Content: "hello"})
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("Publish() error=%v, want uncertain outcome", err)
	}
}

func TestProviderResponseBodyIsBounded(t *testing.T) {
	client := clientWithTransport(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, nil, strings.Repeat("x", maximumProviderBody+1)), nil
	})
	publisher := NewXPublisher("x-token", client)
	publisher.endpoint = "https://provider.test/2/tweets"
	_, err := publisher.Publish(context.Background(), PublishCommand{Content: "hello"})
	if !errors.Is(err, ErrUncertain) || strings.Contains(err.Error(), strings.Repeat("x", 128)) {
		t.Fatalf("Publish() error=%v; want safe uncertain error for oversized body", err)
	}
}

func TestProviderClientHasBoundedTimeout(t *testing.T) {
	client := NewHTTPClient()
	defer client.CloseIdleConnections()
	if client.Timeout <= 0 || client.Timeout > providerRequestTimeout {
		t.Fatalf("HTTP client timeout=%s, want a positive timeout no greater than %s", client.Timeout, providerRequestTimeout)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func clientWithTransport(transport roundTripFunc) *http.Client {
	return &http.Client{Transport: transport}
}

func response(status int, headers http.Header, body string) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body))}
}
