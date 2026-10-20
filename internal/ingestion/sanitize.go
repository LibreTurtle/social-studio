package ingestion

import (
	"strings"
	"unicode"
)

func sanitizeContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	var clean strings.Builder
	clean.Grow(len(content))
	for _, character := range content {
		if character == '\n' || character == '\t' || !unicode.IsControl(character) && !isUnsafeInvisible(character) {
			clean.WriteRune(character)
		}
	}
	return strings.TrimSpace(clean.String())
}

func isUnsafeInvisible(character rune) bool {
	return unicode.Is(unicode.Cf, character) && character != '\u200c' && character != '\u200d'
}
