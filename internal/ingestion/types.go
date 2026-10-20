package ingestion

import (
	"context"
	"errors"
	"net/url"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

var (
	ErrInvalidInput     = errors.New("invalid ingestion input")
	ErrFetchFailed      = errors.New("source fetch failed")
	ErrExtractionFailed = errors.New("source extraction failed")
)

type FetchedPage struct {
	Body        []byte
	FinalURL    *url.URL
	MediaType   string
	ContentType string
}

type Fetcher interface {
	Fetch(context.Context, *url.URL) (FetchedPage, error)
}
type Extractor interface {
	Extract([]byte, string) (string, error)
}

type Result struct {
	SourceType domain.SourceType
	SourceURL  string
	Content    string
}

type InputError struct{ Code string }

func (err *InputError) Error() string        { return err.Code }
func (err *InputError) ErrorCode() string    { return err.Code }
func (err *InputError) Is(target error) bool { return target == ErrInvalidInput }
