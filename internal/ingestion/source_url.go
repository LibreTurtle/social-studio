package ingestion

import (
	"errors"
	"net/url"
	"strings"
)

func normalizeSourceURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return nil, errors.Join(ErrInvalidInput, errors.New("source URL must use HTTP or HTTPS and must not include credentials"))
	}
	if err := validateSourceURL(parsed); err != nil {
		return nil, errors.Join(ErrInvalidInput, err)
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed, nil
}
