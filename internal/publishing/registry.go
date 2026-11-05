package publishing

import (
	"net/http"

	"github.com/LibreTurtle/social-studio/internal/config"
	"github.com/LibreTurtle/social-studio/internal/domain"
)

// NewPublishers assembles the enabled platform strategies from runtime config.
func NewPublishers(settings config.PublishingSettings, client *http.Client) map[domain.Platform]SocialPublisher {
	strategies := []struct {
		platform domain.Platform
		enabled  bool
		create   func() SocialPublisher
	}{
		{
			platform: domain.PlatformX,
			enabled:  settings.X.Enabled,
			create:   func() SocialPublisher { return NewXPublisher(settings.X.AccessToken, client) },
		},
		{
			platform: domain.PlatformLinkedIn,
			enabled:  settings.LinkedIn.Enabled,
			create: func() SocialPublisher {
				return NewLinkedInPublisher(settings.LinkedIn.AccessToken, settings.LinkedIn.AuthorURN, settings.LinkedIn.APIVersion, client)
			},
		},
		{
			platform: domain.PlatformTelegram,
			enabled:  settings.Telegram.Enabled,
			create: func() SocialPublisher {
				return NewTelegramPublisher(settings.Telegram.BotToken, settings.Telegram.ChatID, client)
			},
		},
	}

	publishers := make(map[domain.Platform]SocialPublisher, len(strategies))
	for _, strategy := range strategies {
		if strategy.enabled {
			publishers[strategy.platform] = strategy.create()
		}
	}
	return publishers
}
