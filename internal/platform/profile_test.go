package platform

import "testing"

func TestDefaultProfilesCoverJavaTargets(t *testing.T) {
	profiles := DefaultProfiles()
	if len(profiles) != 3 {
		t.Fatalf("got %d profiles, want 3", len(profiles))
	}
	for _, name := range []Name{X, LinkedIn, Telegram} {
		if _, ok := profiles[name]; !ok {
			t.Errorf("missing profile for %s", name)
		}
	}
}

func TestValidateEnforcesLengthAndHashtagCount(t *testing.T) {
	profile := Profile{Name: X, MaxLength: 20, MaxHashtags: 1, Tone: Concise}
	if violations := Validate("A short post #hello", profile, 1); len(violations) != 0 {
		t.Fatalf("valid content rejected: %v", violations)
	}
	violations := Validate("This is much too long #one #two", profile, 1)
	if len(violations) != 3 || violations[0].Code != "MAX_LENGTH" || violations[1].Code != "MAX_HASHTAGS" {
		t.Fatalf("got %v, want length, hashtag maximum, and requested hashtag count violations", violations)
	}
}

func TestValidateRejectsBlankAndWrongRequestedHashtagCount(t *testing.T) {
	profile := Profile{Name: Telegram, MaxLength: 100, MaxHashtags: 3, Tone: Conversational}
	if got := Validate("  ", profile, 0); len(got) == 0 || got[0].Code != "CONTENT_REQUIRED" {
		t.Fatalf("blank content violations = %v", got)
	}
	if got := Validate("A post #tag", profile, 0); len(got) == 0 || got[0].Code != "WRONG_HASHTAG_COUNT" {
		t.Fatalf("hashtag violations = %v", got)
	}
}

func TestValidateCountsPlatformCharactersAndHashtagsStrictly(t *testing.T) {
	profile := Profile{Name: X, MaxLength: 2, MaxHashtags: 1, Tone: Concise}
	if got := Validate("😀x", profile, 0); len(got) == 0 || got[0].Code != "MAX_LENGTH" {
		t.Fatalf("UTF-16 length violations = %v, want MAX_LENGTH", got)
	}
	if got := Validate("word#tag", Profile{Name: X, MaxLength: 100, MaxHashtags: 1}, 0); len(got) != 0 {
		t.Fatalf("embedded #tag should not count as a hashtag: %v", got)
	}
	if got := Validate("#tag", Profile{Name: X, MaxLength: 100, MaxHashtags: 1}, 1); len(got) != 0 {
		t.Fatalf("standalone hashtag rejected: %v", got)
	}
}
