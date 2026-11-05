package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/LibreTurtle/social-studio/internal/ai"
	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/platform"
	"github.com/LibreTurtle/social-studio/internal/post"
	"github.com/LibreTurtle/social-studio/internal/variant"
)

type postStore struct {
	value post.Post
	err   error
}

func (store postStore) Create(context.Context, post.Post) (post.Post, error) {
	return post.Post{}, errors.New("unexpected Create")
}
func (store postStore) Get(_ context.Context, id string) (post.Post, error) {
	if store.err != nil {
		return post.Post{}, store.err
	}
	if id != store.value.ID {
		return post.Post{}, ErrPostNotFound
	}
	return store.value, nil
}

type draftStore struct {
	created []variant.Draft
	err     error
}

func (store *draftStore) CreateDrafts(_ context.Context, postID string, drafts []variant.Draft) ([]variant.Variant, error) {
	store.created = append([]variant.Draft(nil), drafts...)
	if store.err != nil {
		return nil, store.err
	}
	rows := make([]variant.Variant, len(drafts))
	for index, draft := range drafts {
		rows[index] = variant.Variant{ID: fmt.Sprintf("019a0000-0000-7000-8000-%012d", index+1), PostID: postID, Platform: draft.Platform, Content: draft.Content, Status: domain.VariantStatusDraft, Version: 1}
	}
	return rows, nil
}

type generator struct {
	prompt            ai.Prompt
	content, provider string
	err               error
	contents          []string
	prompts           []ai.Prompt
}

func (fake *generator) Generate(_ context.Context, prompt ai.Prompt) (string, string, error) {
	fake.prompt = prompt
	fake.prompts = append(fake.prompts, prompt)
	if len(fake.contents) > 0 {
		content := fake.contents[0]
		fake.contents = fake.contents[1:]
		return content, fake.provider, fake.err
	}
	return fake.content, fake.provider, fake.err
}

func TestServiceGeneratesOnlyFromStoredPostAndReturnsSession(t *testing.T) {
	stored := post.Post{ID: "018b2f4c-7a00-7000-8000-000000000001", SourceType: domain.SourceTypeMarkdown, Content: "Stored source body"}
	fakeAI := &generator{content: "Generated #Go", provider: "groq"}
	service := NewService(postStore{value: stored}, fakeAI, &draftStore{}, time.Minute, 10)
	result, err := service.Generate(context.Background(), Request{PostID: stored.ID, Preferences: []Preference{{Platform: domain.PlatformLinkedIn, Tone: domain.ToneProfessional, TargetLength: 500, HashtagCount: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if fakeAI.prompt.Source != stored.Content || fakeAI.prompt.Platform != domain.PlatformLinkedIn || fakeAI.prompt.Tone != domain.ToneProfessional {
		t.Fatalf("AI prompt did not use stored post and requested preferences: %+v", fakeAI.prompt)
	}
	if result.PostID != stored.ID || result.ID == "" || len(result.Candidates) != 1 || result.Candidates[0].Content != "Generated #Go" || result.Candidates[0].ID != "019a0000-0000-7000-8000-000000000001" || result.Candidates[0].Status != domain.VariantStatusDraft {
		t.Fatalf("unexpected generation result: %+v", result)
	}
	if _, err := service.Get(result.ID); err != nil {
		t.Fatalf("session not retrievable: %v", err)
	}
}

func TestServiceDoesNotGenerateForMissingPost(t *testing.T) {
	fakeAI := &generator{content: "unexpected"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000002"}}, fakeAI, &draftStore{}, time.Minute, 10)
	_, err := service.Generate(context.Background(), Request{PostID: "018b2f4c-7a00-7000-8000-000000000003", Preferences: []Preference{{Platform: domain.PlatformX, Tone: domain.ToneConcise, TargetLength: 100}}})
	if !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Generate() error = %v, want post not found", err)
	}
	if fakeAI.prompt.Source != "" {
		t.Fatalf("AI was called for missing post: %+v", fakeAI.prompt)
	}
}

func TestServiceRejectsInvalidPreferencesBeforeCallingAI(t *testing.T) {
	fakeAI := &generator{content: "Generated"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000004", Content: "body"}}, fakeAI, &draftStore{}, time.Minute, 10)
	_, err := service.Generate(context.Background(), Request{PostID: "018b2f4c-7a00-7000-8000-000000000004", Preferences: []Preference{{Platform: domain.Platform("UNKNOWN"), Tone: platform.Concise, TargetLength: 50}}})
	if err == nil {
		t.Fatal("Generate() accepted invalid platform")
	}
	if fakeAI.prompt.Source != "" {
		t.Fatalf("AI called before validation: %+v", fakeAI.prompt)
	}
}

func TestServiceRegeneratesInvalidOutputBeforeKeepingCandidate(t *testing.T) {
	tooLong := strings.Repeat("x", 281)
	fakeAI := &generator{contents: []string{tooLong, "A valid post"}, provider: "gemini"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000005", Content: "body"}}, fakeAI, &draftStore{}, time.Minute, 10)
	result, err := service.Generate(context.Background(), Request{PostID: "018b2f4c-7a00-7000-8000-000000000005", Preferences: []Preference{{Platform: domain.PlatformX, Tone: domain.ToneConcise, TargetLength: 280, HashtagCount: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(fakeAI.prompts) != 2 || len(fakeAI.prompts[1].ConstraintFeedback) == 0 {
		t.Fatalf("invalid content was not followed by a correction request: %+v", fakeAI.prompts)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Content != "A valid post" || len(result.Candidates[0].Violations) != 0 {
		t.Fatalf("invalid candidate escaped validation: %+v", result.Candidates)
	}
	if _, err := service.Get(result.ID); err != nil {
		t.Fatalf("valid result was not cached: %v", err)
	}
}

func TestServiceFailsClosedAfterRepeatedInvalidOutput(t *testing.T) {
	fakeAI := &generator{content: strings.Repeat("x", 281), provider: "gemini"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000005", Content: "body"}}, fakeAI, &draftStore{}, time.Minute, 10)
	result, err := service.Generate(context.Background(), Request{PostID: "018b2f4c-7a00-7000-8000-000000000005", Preferences: []Preference{{Platform: domain.PlatformX, Tone: domain.ToneConcise, TargetLength: 280, HashtagCount: 0}}})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Generate() error = %v, want ErrInvalidOutput", err)
	}
	if len(result.Candidates) != 0 || len(fakeAI.prompts) != MaxGenerationAttempts {
		t.Fatalf("invalid output escaped or attempts were not bounded: result=%+v attempts=%d", result, len(fakeAI.prompts))
	}
	if _, err := service.Get(result.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("invalid generation session was cached: %v", err)
	}
}

func TestServicePersistsValidatedDraftsBeforeCachingSession(t *testing.T) {
	storeErr := errors.New("variant persistence failed")
	fakeStore := &draftStore{err: storeErr}
	fakeAI := &generator{content: "A valid post"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000005", Content: "body"}}, fakeAI, fakeStore, time.Minute, 10)
	result, err := service.Generate(context.Background(), Request{PostID: "018b2f4c-7a00-7000-8000-000000000005", Preferences: []Preference{{Platform: domain.PlatformX, Tone: domain.ToneConcise, TargetLength: 280}}})
	if !errors.Is(err, storeErr) {
		t.Fatalf("Generate() error = %v, want persistence error", err)
	}
	if len(result.Candidates) != 0 || len(fakeStore.created) != 1 {
		t.Fatalf("failed persistence exposed candidates: result=%+v drafts=%+v", result, fakeStore.created)
	}
	if _, err := service.Get(result.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("failed persistence cached generation session: %v", err)
	}
}

func TestServiceDoesNotRetryWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fakeAI := &generator{content: strings.Repeat("x", 281), provider: "gemini"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000005", Content: "body"}}, fakeAI, &draftStore{}, time.Minute, 10)
	// Cancel from the first provider call to model a client disconnect during regeneration.
	wrapped := cancelingGenerator{generator: fakeAI, cancel: cancel}
	service.generator = wrapped
	_, err := service.Generate(ctx, Request{PostID: "018b2f4c-7a00-7000-8000-000000000005", Preferences: []Preference{{Platform: domain.PlatformX, Tone: domain.ToneConcise, TargetLength: 280}}})
	if err == nil || len(fakeAI.prompts) != 1 {
		t.Fatalf("canceled generation retried: err=%v attempts=%d", err, len(fakeAI.prompts))
	}
}

type cancelingGenerator struct {
	generator *generator
	cancel    context.CancelFunc
}

func (fake cancelingGenerator) Generate(ctx context.Context, prompt ai.Prompt) (string, string, error) {
	content, provider, err := fake.generator.Generate(ctx, prompt)
	fake.cancel()
	return content, provider, err
}

func TestServiceKeepsSessionCandidateForValidContent(t *testing.T) {
	fakeAI := &generator{content: "A valid post", provider: "gemini"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000005", Content: "body"}}, fakeAI, &draftStore{}, time.Minute, 10)
	result, err := service.Generate(context.Background(), Request{PostID: "018b2f4c-7a00-7000-8000-000000000005", Preferences: []Preference{{Platform: domain.PlatformX, Tone: domain.ToneConcise, TargetLength: 100, HashtagCount: 0}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates[0].Violations) != 0 {
		t.Fatalf("unexpected violations: %+v", result.Candidates[0].Violations)
	}
	if _, err := service.Candidates(result.ID); err != nil {
		t.Fatal(err)
	}
}

func TestServiceExpiresSessions(t *testing.T) {
	fakeAI := &generator{content: "A valid post"}
	service := NewService(postStore{value: post.Post{ID: "018b2f4c-7a00-7000-8000-000000000006", Content: "body"}}, fakeAI, &draftStore{}, time.Nanosecond, 10)
	result, err := service.Generate(context.Background(), Request{PostID: "018b2f4c-7a00-7000-8000-000000000006", Preferences: []Preference{{Platform: domain.PlatformX, Tone: domain.ToneConcise, TargetLength: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if _, err := service.Get(result.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("Get() error = %v, want expired session", err)
	}
}
