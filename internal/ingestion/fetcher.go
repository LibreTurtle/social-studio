package ingestion

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

const maxPageBytes = 2 << 20

type HTTPFetcher struct {
	resolver     ipResolver
	timeout      time.Duration
	maxRedirects int
}

type ipResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

func NewHTTPFetcher() *HTTPFetcher { return newHTTPFetcher(net.DefaultResolver) }

func newHTTPFetcher(resolver ipResolver) *HTTPFetcher {
	return &HTTPFetcher{resolver: resolver, timeout: 15 * time.Second, maxRedirects: 5}
}

func (fetcher *HTTPFetcher) Fetch(ctx context.Context, source *url.URL) (FetchedPage, error) {
	if source == nil {
		return FetchedPage{}, errors.New("source URL is required")
	}
	current := *source
	for redirects := 0; ; redirects++ {
		if err := validateSourceURL(&current); err != nil {
			return FetchedPage{}, err
		}
		addresses, err := resolvePublicAddresses(ctx, fetcher.resolver, current.Hostname())
		if err != nil {
			return FetchedPage{}, err
		}
		transport := newPinnedTransport(addresses, current)
		client := &http.Client{Transport: transport, Timeout: fetcher.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
		if err != nil {
			transport.CloseIdleConnections()
			return FetchedPage{}, errors.New("invalid source request")
		}
		request.Header.Set("User-Agent", "SocialStudio/1.0 (+content ingestion)")
		request.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.8")
		response, err := client.Do(request)
		if err != nil {
			transport.CloseIdleConnections()
			return FetchedPage{}, errors.New("source request failed")
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			next, redirectErr := redirectTarget(current, response, redirects, fetcher.maxRedirects)
			transport.CloseIdleConnections()
			if redirectErr != nil {
				return FetchedPage{}, redirectErr
			}
			current = *next
			continue
		}
		page, fetchErr := readPage(response, current)
		transport.CloseIdleConnections()
		return page, fetchErr
	}
}

func redirectTarget(current url.URL, response *http.Response, redirects, maxRedirects int) (*url.URL, error) {
	defer response.Body.Close()
	location := response.Header.Get("Location")
	if location == "" || redirects >= maxRedirects {
		return nil, errors.New("source redirect limit exceeded or location missing")
	}
	next, err := current.Parse(location)
	if err != nil {
		return nil, errors.New("source returned an invalid redirect")
	}
	if current.Scheme == "https" && next.Scheme != "https" {
		return nil, errors.New("HTTPS source cannot redirect to HTTP")
	}
	if err := validateSourceURL(next); err != nil {
		return nil, err
	}
	return next, nil
}

func readPage(response *http.Response, current url.URL) (FetchedPage, error) {
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return FetchedPage{}, fmt.Errorf("source returned HTTP %d", response.StatusCode)
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" && mediaType != "text/plain" {
		return FetchedPage{}, errors.New("source content type is not supported")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageBytes+1))
	if err != nil || len(body) > maxPageBytes {
		return FetchedPage{}, errors.New("source response exceeds size limit or could not be read")
	}
	return FetchedPage{Body: body, FinalURL: &current, MediaType: mediaType, ContentType: response.Header.Get("Content-Type")}, nil
}
