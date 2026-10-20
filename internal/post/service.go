package post

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/ingestion"
)

var ErrNotFound = errors.New("post not found")

type Post struct {
	ID         string            `json:"id"`
	SourceType domain.SourceType `json:"source_type"`
	SourceURL  string            `json:"source_url,omitempty"`
	Content    string            `json:"content"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	Version    int64             `json:"version"`
}
type CreateRequest struct {
	SourceType domain.SourceType `json:"source_type"`
	SourceURL  string            `json:"source_url,omitempty"`
	Content    string            `json:"content,omitempty"`
}
type Store interface {
	Create(context.Context, Post) (Post, error)
	Get(context.Context, string) (Post, error)
}
type Reader interface {
	Read(context.Context, domain.SourceType, string, string) (ingestion.Result, error)
}
type Service struct {
	store  Store
	reader Reader
}

func NewService(store Store, reader Reader) *Service { return &Service{store: store, reader: reader} }

func (service *Service) Create(ctx context.Context, request CreateRequest) (Post, error) {
	result, err := service.reader.Read(ctx, request.SourceType, request.SourceURL, request.Content)
	if err != nil {
		return Post{}, err
	}
	return service.store.Create(ctx, Post{SourceType: result.SourceType, SourceURL: result.SourceURL, Content: result.Content})
}

func (service *Service) Get(ctx context.Context, id string) (Post, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Post{}, ErrNotFound
	}
	return service.store.Get(ctx, id)
}
