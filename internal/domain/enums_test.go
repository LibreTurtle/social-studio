package domain

import "testing"

func TestEnumValuesMatchSupportedContracts(t *testing.T) {
	tests := []struct {
		name   string
		values []bool
	}{
		{
			name: "platform",
			values: []bool{
				PlatformX.Valid(), PlatformLinkedIn.Valid(), PlatformTelegram.Valid(),
				Platform("MASTODON").Valid(),
			},
		},
		{
			name: "tone",
			values: []bool{
				ToneConcise.Valid(), ToneProfessional.Valid(), ToneConversational.Valid(),
				Tone("CASUAL").Valid(),
			},
		},
		{
			name:   "source type",
			values: []bool{SourceTypeURL.Valid(), SourceTypeMarkdown.Valid(), SourceType("HTML").Valid()},
		},
		{
			name: "variant status",
			values: []bool{
				VariantStatusDraft.Valid(), VariantStatusApproved.Valid(), VariantStatusRejected.Valid(),
				VariantStatusPublished.Valid(), VariantStatus("PENDING").Valid(),
			},
		},
		{
			name: "schedule status",
			values: []bool{
				ScheduleStatusPending.Valid(), ScheduleStatusProcessing.Valid(),
				ScheduleStatusCompleted.Valid(), ScheduleStatusFailed.Valid(), ScheduleStatus("DRAFT").Valid(),
			},
		},
		{
			name: "publish attempt status",
			values: []bool{
				PublishAttemptStatusSuccess.Valid(), PublishAttemptStatusFailed.Valid(),
				PublishAttemptStatus("PENDING").Valid(),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for index, got := range test.values {
				want := index < len(test.values)-1
				if got != want {
					t.Fatalf("validity at index %d = %t, want %t", index, got, want)
				}
			}
		})
	}
}
