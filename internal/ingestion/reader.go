package ingestion

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

type Reader struct {
	fetcher   Fetcher
	extractor Extractor
}

func NewReader(fetcher Fetcher, extractor Extractor) *Reader {
	return &Reader{fetcher: fetcher, extractor: extractor}
}

func (reader *Reader) Read(ctx context.Context, sourceType domain.SourceType, sourceURL, content string) (Result, error) {
	switch sourceType {
	case domain.SourceTypeMarkdown:
		if strings.TrimSpace(sourceURL) != "" {
			return Result{}, &InputError{Code: "SOURCE_URL_NOT_ALLOWED_FOR_MARKDOWN"}
		}
		content = strings.TrimSpace(content)
		if content == "" {
			return Result{}, &InputError{Code: "MARKDOWN_CONTENT_REQUIRED"}
		}
		if len(content) > 2<<20 {
			return Result{}, &InputError{Code: "MARKDOWN_TOO_LARGE"}
		}
		if !utf8.ValidString(content) {
			return Result{}, &InputError{Code: "MARKDOWN_INVALID_UTF8"}
		}
		content = sanitizeContent(content)
		if content == "" {
			return Result{}, &InputError{Code: "MARKDOWN_CONTENT_REQUIRED"}
		}
		return Result{SourceType: sourceType, Content: content}, nil
	case domain.SourceTypeURL:
		if strings.TrimSpace(content) != "" {
			return Result{}, &InputError{Code: "CONTENT_NOT_ALLOWED_FOR_URL"}
		}
		if strings.TrimSpace(sourceURL) == "" {
			return Result{}, &InputError{Code: "SOURCE_URL_REQUIRED"}
		}
		parsed, err := normalizeSourceURL(sourceURL)
		if err != nil {
			return Result{}, &InputError{Code: "INVALID_SOURCE_URL"}
		}
		if reader.fetcher == nil || reader.extractor == nil {
			return Result{}, fmt.Errorf("%w: URL ingestion is not configured", ErrInvalidInput)
		}
		page, err := reader.fetcher.Fetch(ctx, parsed)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrFetchFailed, err)
		}
		contentType := page.ContentType
		if contentType == "" {
			contentType = page.MediaType
		}
		text, err := reader.extractor.Extract(page.Body, contentType)
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrExtractionFailed, err)
		}
		text = sanitizeContent(text)
		if text == "" {
			return Result{}, fmt.Errorf("%w: source page contained no readable text", ErrExtractionFailed)
		}
		finalURL := parsed
		if page.FinalURL != nil {
			finalURL = page.FinalURL
		}
		return Result{SourceType: sourceType, SourceURL: finalURL.String(), Content: text}, nil
	default:
		return Result{}, &InputError{Code: "UNSUPPORTED_SOURCE_TYPE"}
	}
}
