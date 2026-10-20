package platform

import (
	"fmt"
	"regexp"
	"unicode"
	"unicode/utf16"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

type Name = domain.Platform
type Tone = domain.Tone

const (
	X        Name = domain.PlatformX
	LinkedIn Name = domain.PlatformLinkedIn
	Telegram Name = domain.PlatformTelegram

	Concise        Tone = domain.ToneConcise
	Professional   Tone = domain.ToneProfessional
	Conversational Tone = domain.ToneConversational
)

type Profile struct {
	Name        Name `json:"name"`
	MaxLength   int  `json:"max_length"`
	MaxHashtags int  `json:"max_hashtags"`
	Tone        Tone `json:"tone"`
}

func DefaultProfiles() map[Name]Profile {
	profiles := []Profile{
		{Name: X, MaxLength: 280, MaxHashtags: 3, Tone: Concise},
		{Name: LinkedIn, MaxLength: 3000, MaxHashtags: 5, Tone: Professional},
		{Name: Telegram, MaxLength: 4096, MaxHashtags: 5, Tone: Conversational},
	}
	result := make(map[Name]Profile, len(profiles))
	for _, profile := range profiles {
		result[profile.Name] = profile
	}
	return result
}

type Violation struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var hashtagPattern = regexp.MustCompile(`#\p{L}[\p{L}\p{N}_]*`)

func Validate(content string, profile Profile, requiredHashtags int) []Violation {
	if len([]rune(content)) == 0 || allWhitespace(content) {
		return []Violation{{Code: "CONTENT_REQUIRED", Message: "Generated content must not be blank"}}
	}
	var violations []Violation
	length := len(utf16.Encode([]rune(content)))
	if length > profile.MaxLength {
		violations = append(violations, Violation{Code: "MAX_LENGTH", Message: fmt.Sprintf("Content has %d characters; maximum allowed is %d", length, profile.MaxLength)})
	}
	count := 0
	for _, match := range hashtagPattern.FindAllStringIndex(content, -1) {
		if match[0] > 0 && isASCIIWord(content[match[0]-1]) {
			continue
		}
		count++
	}
	if count > profile.MaxHashtags {
		violations = append(violations, Violation{Code: "MAX_HASHTAGS", Message: fmt.Sprintf("Content has %d hashtags; maximum allowed is %d", count, profile.MaxHashtags)})
	}
	if count != requiredHashtags {
		violations = append(violations, Violation{Code: "WRONG_HASHTAG_COUNT", Message: fmt.Sprintf("Content contains %d hashtags; requested count is %d", count, requiredHashtags)})
	}
	return violations
}

func isASCIIWord(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_'
}

func allWhitespace(value string) bool {
	for _, r := range value {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
