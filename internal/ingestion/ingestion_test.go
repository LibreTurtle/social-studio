package ingestion

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

type fetcherFunc func(context.Context, *url.URL) (FetchedPage, error)

func (function fetcherFunc) Fetch(ctx context.Context, source *url.URL) (FetchedPage, error) {
	return function(ctx, source)
}

type extractorFunc func([]byte, string) (string, error)

func (function extractorFunc) Extract(body []byte, mediaType string) (string, error) {
	return function(body, mediaType)
}

func TestReaderAcceptsAndNormalizesMarkdown(t *testing.T) {
	result, err := NewReader(nil, nil).Read(context.Background(), domain.SourceTypeMarkdown, "", "  # Heading\n\nText  ")
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "# Heading\n\nText" || result.SourceURL != "" || result.SourceType != domain.SourceTypeMarkdown {
		t.Fatalf("unexpected ingestion result: %+v", result)
	}
}

func TestReaderRemovesUnsafeControlAndBidirectionalCharacters(t *testing.T) {
	result, err := NewReader(nil, nil).Read(context.Background(), domain.SourceTypeMarkdown, "", "Visible\x00 text\u202e reversed\r\nnext\u200bword\u2060here")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Visible text reversed\nnextwordhere"; result.Content != want {
		t.Fatalf("sanitized content = %q, want %q", result.Content, want)
	}
}

func TestReaderRejectsMissingAndConflictingSourceFields(t *testing.T) {
	tests := []struct {
		name                         string
		sourceType                   domain.SourceType
		sourceURL, content, wantCode string
	}{
		{name: "missing markdown", sourceType: domain.SourceTypeMarkdown, wantCode: "MARKDOWN_CONTENT_REQUIRED"},
		{name: "markdown with URL", sourceType: domain.SourceTypeMarkdown, sourceURL: "https://example.com", content: "text", wantCode: "SOURCE_URL_NOT_ALLOWED_FOR_MARKDOWN"},
		{name: "missing URL", sourceType: domain.SourceTypeURL, wantCode: "SOURCE_URL_REQUIRED"},
		{name: "URL with markdown", sourceType: domain.SourceTypeURL, sourceURL: "https://example.com", content: "text", wantCode: "CONTENT_NOT_ALLOWED_FOR_URL"},
		{name: "unknown source type", sourceType: domain.SourceType("HTML"), wantCode: "UNSUPPORTED_SOURCE_TYPE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewReader(nil, nil).Read(context.Background(), test.sourceType, test.sourceURL, test.content)
			problem, ok := err.(interface{ ErrorCode() string })
			if !ok || problem.ErrorCode() != test.wantCode {
				t.Fatalf("Read() error = %v, want code %q", err, test.wantCode)
			}
		})
	}
}

func TestReaderFetchesURLAndPersistsExtractedFinalContent(t *testing.T) {
	finalURL, err := url.Parse("https://example.com/article")
	if err != nil {
		t.Fatal(err)
	}
	var requestedURL string
	reader := NewReader(fetcherFunc(func(_ context.Context, source *url.URL) (FetchedPage, error) {
		requestedURL = source.String()
		return FetchedPage{Body: []byte("<article>article</article>"), FinalURL: finalURL, MediaType: "text/html"}, nil
	}), extractorFunc(func(body []byte, mediaType string) (string, error) {
		if string(body) != "<article>article</article>" || mediaType != "text/html" {
			t.Fatalf("unexpected extractor input: %q, %q", body, mediaType)
		}
		return "Extracted article text", nil
	}))
	result, err := reader.Read(context.Background(), domain.SourceTypeURL, " example.com/article/ ", "")
	if err != nil {
		t.Fatal(err)
	}
	if requestedURL != "https://example.com/article" {
		t.Fatalf("fetch URL = %q", requestedURL)
	}
	if result.SourceURL != finalURL.String() || result.Content != "Extracted article text" {
		t.Fatalf("unexpected ingestion result: %+v", result)
	}
}

func TestReaderPropagatesRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := NewReader(fetcherFunc(func(ctx context.Context, _ *url.URL) (FetchedPage, error) { return FetchedPage{}, ctx.Err() }), extractorFunc(func([]byte, string) (string, error) { return "", nil }))
	if _, err := reader.Read(ctx, domain.SourceTypeURL, "https://example.com", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read() error = %v, want context.Canceled", err)
	}
}

func TestHTMLExtractorPrefersArticleAndRemovesNonContent(t *testing.T) {
	page := []byte(`<html><body><nav>Navigation</nav><main>Fallback body</main><article><header><h1>Title &amp; intro</h1></header><p>First paragraph.</p><script>secret()</script><p aria-hidden="true">Hidden text</p><p>Second <strong>paragraph</strong>.</p></article></body></html>`)
	content, err := NewHTMLExtractor().Extract(page, "text/html")
	if err != nil {
		t.Fatal(err)
	}
	if content != "Title & intro\n\nFirst paragraph.\n\nSecond paragraph." {
		t.Fatalf("extracted text = %q", content)
	}
	if strings.Contains(content, "Navigation") || strings.Contains(content, "secret") || strings.Contains(content, "Hidden") {
		t.Fatalf("non-content text leaked: %q", content)
	}
}

func TestHTMLExtractorDecodesDeclaredCharacterSet(t *testing.T) {
	page := []byte(`<article>Caf` + string([]byte{0xe9}) + `</article>`)
	content, err := NewHTMLExtractor().Extract(page, "text/html; charset=windows-1252")
	if err != nil {
		t.Fatal(err)
	}
	if content != "Café" {
		t.Fatalf("decoded text = %q, want Café", content)
	}
}

func TestHTMLExtractorFallsBackToMainThenBody(t *testing.T) {
	for _, test := range []struct{ html, want string }{
		{`<html><body><main><p>Main content</p></main><div>Other</div></body></html>`, "Main content"},
		{`<html><body><div>Body content</div><footer>Footer</footer></body></html>`, "Body content"},
	} {
		got, err := NewHTMLExtractor().Extract([]byte(test.html), "text/html")
		if err != nil || got != test.want {
			t.Fatalf("Extract() = (%q, %v), want %q", got, err, test.want)
		}
	}
}

func TestHTMLExtractorSupportsPlainTextAndRejectsOtherMediaTypes(t *testing.T) {
	content, err := NewHTMLExtractor().Extract([]byte("  plain text  "), "text/plain")
	if err != nil || content != "plain text" {
		t.Fatalf("Extract() = (%q, %v)", content, err)
	}
	if _, err := NewHTMLExtractor().Extract(nil, "application/pdf"); err != ErrUnsupportedMediaType {
		t.Fatalf("unsupported media error = %v", err)
	}
}

func TestHTMLExtractorRejectsOversizedInput(t *testing.T) {
	if _, err := NewHTMLExtractor().Extract(make([]byte, maxPageBytes+1), "text/html"); err == nil {
		t.Fatal("Extract() accepted oversized HTML")
	}
}

func TestPublicAddressFilterBlocksNonRoutableAndSpecialRanges(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.0.1", "::1", "fc00::1", "fe80::1", "2001:db8::1", "::ffff:127.0.0.1"} {
		address, err := netip.ParseAddr(value)
		if err != nil {
			t.Fatal(err)
		}
		if isPublic(address) {
			t.Errorf("isPublic(%s) = true, want false", value)
		}
	}
	for _, value := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		address, err := netip.ParseAddr(value)
		if err != nil {
			t.Fatal(err)
		}
		if !isPublic(address) {
			t.Errorf("isPublic(%s) = false, want true", value)
		}
	}
}

func TestValidateSourceURLRejectsNonHTTPAndCredentials(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com/file", "https://user:password@example.com", "https://example.com:99999"} {
		source, _ := url.Parse(raw)
		if err := validateSourceURL(source); err == nil {
			t.Errorf("validateSourceURL(%q) succeeded", raw)
		}
	}
}

func TestNormalizeSourceURLAddsHTTPSAndStripsFragmentAndTrailingSlash(t *testing.T) {
	source, err := normalizeSourceURL("  Example.COM/article/#section  ")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := source.String(), "https://example.com/article"; got != want {
		t.Fatalf("normalized URL = %q, want %q", got, want)
	}
}

func TestNormalizeSourceURLRejectsDisallowedPorts(t *testing.T) {
	if _, err := normalizeSourceURL("https://example.com:8443/article"); err == nil {
		t.Fatal("normalizeSourceURL() accepted a non-standard port")
	}
}
