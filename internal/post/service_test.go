package post

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
)

type fakeStore struct {
	created   Post
	createErr error
}

func (store *fakeStore) Create(_ context.Context, value Post) (Post, error) {
	store.created = value
	return value, store.createErr
}

func (store *fakeStore) Get(context.Context, string) (Post, error) {
	return Post{}, errors.New("unexpected Get")
}

type fakeReader struct {
	gotType    domain.SourceType
	gotURL     string
	gotContent string
	result     ingestion.Result
	err        error
}

func (reader *fakeReader) Read(_ context.Context, sourceType domain.SourceType, sourceURL, content string) (ingestion.Result, error) {
	reader.gotType, reader.gotURL, reader.gotContent = sourceType, sourceURL, content
	return reader.result, reader.err
}

func TestServiceCreatesPostFromReaderOutput(t *testing.T) {
	finalURL, err := url.Parse("https://example.com/article")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{}
	reader := &fakeReader{result: ingestion.Result{
		SourceType: domain.SourceTypeURL,
		SourceURL:  finalURL.String(),
		Content:    "Extracted, normalized article content",
	}}
	service := NewService(store, reader)

	createdAt := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	store.createErr = nil
	store.created.CreatedAt = createdAt
	post, err := service.Create(context.Background(), CreateRequest{
		SourceType: domain.SourceTypeURL,
		SourceURL:  "https://example.com/article",
		Content:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reader.gotType != domain.SourceTypeURL || reader.gotURL != "https://example.com/article" {
		t.Fatalf("reader received (%q, %q)", reader.gotType, reader.gotURL)
	}
	if store.created.Content != "Extracted, normalized article content" || store.created.SourceURL != finalURL.String() {
		t.Fatalf("stored post did not use reader output: %+v", store.created)
	}
	if post.Content != store.created.Content {
		t.Fatalf("returned post content = %q, want stored reader output", post.Content)
	}
}

func TestServiceDoesNotStoreWhenIngestionFails(t *testing.T) {
	readErr := errors.New("fetch failed")
	store := &fakeStore{}
	reader := &fakeReader{err: readErr}
	_, err := NewService(store, reader).Create(context.Background(), CreateRequest{SourceType: domain.SourceTypeURL})
	if !errors.Is(err, readErr) {
		t.Fatalf("Create() error = %v, want %v", err, readErr)
	}
	if store.created.Content != "" {
		t.Fatalf("store was called after reader failure: %+v", store.created)
	}
}

func TestServicePropagatesCanceledRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := &fakeStore{}
	reader := &fakeReader{err: ctx.Err()}
	_, err := NewService(store, reader).Create(ctx, CreateRequest{SourceType: domain.SourceTypeMarkdown, Content: "some content"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create() error = %v, want context canceled", err)
	}
}
