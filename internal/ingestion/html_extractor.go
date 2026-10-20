package ingestion

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

var ErrUnsupportedMediaType = errors.New("unsupported source media type")

type HTMLExtractor struct{}

func NewHTMLExtractor() *HTMLExtractor { return &HTMLExtractor{} }

func (*HTMLExtractor) Extract(body []byte, contentType string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" && mediaType != "text/plain" {
		return "", ErrUnsupportedMediaType
	}
	if len(body) > maxPageBytes {
		return "", errors.New("source response exceeds size limit")
	}
	decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return "", err
	}
	if mediaType == "text/plain" {
		text, err := io.ReadAll(io.LimitReader(decoded, maxPageBytes+1))
		if err != nil || len(text) > maxPageBytes {
			return "", errors.New("source response exceeds size limit or could not be decoded")
		}
		return strings.TrimSpace(string(text)), nil
	}
	utf8HTML, err := io.ReadAll(io.LimitReader(decoded, maxPageBytes+1))
	if err != nil || len(utf8HTML) > maxPageBytes {
		return "", errors.New("decoded source response exceeds size limit")
	}
	root, err := html.Parse(bytes.NewReader(utf8HTML))
	if err != nil {
		return "", err
	}
	content := findContentRoot(root, "article")
	if content == nil {
		content = findContentRoot(root, "main")
	}
	excludePageChrome := content == nil
	if content == nil {
		content = findContentRoot(root, "body")
	}
	if content == nil {
		content = root
	}
	var output strings.Builder
	appendVisibleText(&output, content, excludePageChrome)
	return normalizeExtractedText(output.String()), nil
}

func findContentRoot(root *html.Node, tag string) *html.Node {
	var found *html.Node
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node == nil || found != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == tag {
			found = node
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return found
}

func appendVisibleText(output *strings.Builder, node *html.Node, excludePageChrome bool) {
	if node == nil {
		return
	}
	if node.Type == html.ElementNode {
		if isExcludedElement(node, excludePageChrome) || hasHiddenAttribute(node) {
			return
		}
		if isBlockElement(node.Data) {
			output.WriteByte('\n')
		}
	}
	if node.Type == html.TextNode {
		appendTextNode(output, node.Data)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendVisibleText(output, child, excludePageChrome)
	}
	if node.Type == html.ElementNode && isBlockElement(node.Data) {
		output.WriteByte('\n')
	}
}

func appendTextNode(output *strings.Builder, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if output.Len() > 0 {
		previous := output.String()[output.Len()-1]
		first := value[0]
		if !isASCIIWhitespace(previous) && !strings.ContainsRune(".,!?;:)]}%", rune(first)) {
			output.WriteByte(' ')
		}
	}
	output.WriteString(value)
}

func isASCIIWhitespace(value byte) bool {
	return value == ' ' || value == '\n' || value == '\t' || value == '\r'
}

func isExcludedElement(node *html.Node, excludePageChrome bool) bool {
	switch node.Data {
	case "script", "style", "noscript", "svg", "nav", "aside", "form", "button", "template", "iframe", "select", "textarea":
		return true
	case "footer", "header":
		return excludePageChrome
	default:
		return false
	}
}

func hasHiddenAttribute(node *html.Node) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == "hidden" || attribute.Key == "aria-hidden" && strings.EqualFold(attribute.Val, "true") {
			return true
		}
		if attribute.Key == "style" {
			style := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(attribute.Val, " ", ""), "\n", ""))
			if strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden") {
				return true
			}
		}
	}
	return false
}

func isBlockElement(tag string) bool {
	switch tag {
	case "address", "article", "blockquote", "br", "dd", "div", "dl", "dt", "figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6", "hr", "li", "main", "ol", "p", "pre", "section", "table", "td", "th", "tr", "ul":
		return true
	default:
		return false
	}
}

func normalizeExtractedText(value string) string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(result) > 0 && result[len(result)-1] != "" {
				result = append(result, "")
			}
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}
