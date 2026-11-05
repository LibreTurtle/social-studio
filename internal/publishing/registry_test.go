package publishing

import (
	"net/http"
	"testing"

	"github.com/LibreTurtle/social-studio/internal/config"
	"github.com/LibreTurtle/social-studio/internal/domain"
)

func TestNewPublishersRegistersOnlyEnabledPlatformStrategies(t *testing.T) {
	settings := config.PublishingSettings{
		X: config.XPublishingSettings{Enabled: true, AccessToken: "x-token"},
		LinkedIn: config.LinkedInPublishingSettings{
			Enabled: true, AccessToken: "linkedin-token", AuthorURN: "urn:li:person:member", APIVersion: "202610",
		},
		Telegram: config.TelegramPublishingSettings{BotToken: "disabled-token", ChatID: "@channel"},
	}

	client := &http.Client{}
	publishers := NewPublishers(settings, client)
	if len(publishers) != 2 {
		t.Fatalf("registered publishers = %d, want 2", len(publishers))
	}
	if _, ok := publishers[domain.PlatformX].(*XPublisher); !ok {
		t.Fatalf("X strategy = %T, want *XPublisher", publishers[domain.PlatformX])
	}
	if _, ok := publishers[domain.PlatformLinkedIn].(*LinkedInPublisher); !ok {
		t.Fatalf("LinkedIn strategy = %T, want *LinkedInPublisher", publishers[domain.PlatformLinkedIn])
	}
	if _, ok := publishers[domain.PlatformTelegram]; ok {
		t.Fatal("disabled Telegram strategy was registered")
	}
}
