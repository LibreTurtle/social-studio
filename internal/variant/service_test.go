package variant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LibreTurtle/social-studio/internal/domain"
)

const testPostID = "018b2f4c-7a00-7000-8000-000000000001"
const testVariantID = "019a0000-0000-7000-8000-000000000001"

type fakeStore struct {
	created         []Draft
	createdPostID   string
	createdResult   []Variant
	createdErr      error
	value           Variant
	getErr          error
	listStatus      domain.VariantStatus
	listLimit       int
	listOffset      int
	listResult      []Variant
	listErr         error
	updatedContent  string
	updatedVersion  int64
	updatedResult   Variant
	updatedErr      error
	approvedVersion int64
	approvedResult  Variant
	approvedErr     error
	rejectedVersion int64
	rejectedResult  Variant
	rejectedErr     error
}

func (store *fakeStore) CreateDrafts(_ context.Context, postID string, drafts []Draft) ([]Variant, error) {
	store.createdPostID = postID
	store.created = drafts
	return store.createdResult, store.createdErr
}
func (store *fakeStore) Get(context.Context, string) (Variant, error) {
	return store.value, store.getErr
}
func (store *fakeStore) ListByPost(_ context.Context, _ string, status domain.VariantStatus, limit, offset int) ([]Variant, error) {
	store.listStatus, store.listLimit, store.listOffset = status, limit, offset
	return store.listResult, store.listErr
}
func (store *fakeStore) UpdateContent(_ context.Context, _ string, version int64, content string) (Variant, error) {
	store.updatedVersion, store.updatedContent = version, content
	return store.updatedResult, store.updatedErr
}
func (store *fakeStore) Approve(_ context.Context, _ string, version int64) (Variant, error) {
	store.approvedVersion = version
	return store.approvedResult, store.approvedErr
}
func (store *fakeStore) Reject(_ context.Context, _ string, version int64) (Variant, error) {
	store.rejectedVersion = version
	return store.rejectedResult, store.rejectedErr
}

func TestCreateDraftsValidatesAndPersistsDrafts(t *testing.T) {
	createdAt := time.Date(2026, time.October, 21, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{createdResult: []Variant{{ID: testVariantID, PostID: testPostID, Platform: domain.PlatformX, Content: "Ready", Status: domain.VariantStatusDraft, CreatedAt: createdAt, UpdatedAt: createdAt, Version: 1}}}
	drafts := []Draft{{Platform: domain.PlatformX, Content: "Ready"}}
	got, err := NewService(store).CreateDrafts(context.Background(), testPostID, drafts)
	if err != nil {
		t.Fatal(err)
	}
	if store.createdPostID != testPostID || len(store.created) != 1 || len(got) != 1 || got[0].Status != domain.VariantStatusDraft {
		t.Fatalf("persisted drafts = %+v, input = %+v", got, store.created)
	}
}

func TestCreateDraftsRejectsInvalidContentBeforePersistence(t *testing.T) {
	store := &fakeStore{}
	_, err := NewService(store).CreateDrafts(context.Background(), testPostID, []Draft{{Platform: domain.PlatformX, Content: ""}})
	if !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("CreateDrafts() error = %v, want ErrInvalidContent", err)
	}
	if store.created != nil {
		t.Fatalf("store called for invalid draft: %+v", store.created)
	}
}

func TestListByPostAppliesValidatedPaginationAndStatus(t *testing.T) {
	store := &fakeStore{listResult: []Variant{}}
	_, err := NewService(store).ListByPost(context.Background(), testPostID, domain.VariantStatusDraft, 25, 50)
	if err != nil {
		t.Fatal(err)
	}
	if store.listStatus != domain.VariantStatusDraft || store.listLimit != 25 || store.listOffset != 50 {
		t.Fatalf("list parameters = status %q, limit %d, offset %d", store.listStatus, store.listLimit, store.listOffset)
	}
	if _, err := NewService(store).ListByPost(context.Background(), testPostID, domain.VariantStatus("UNKNOWN"), 25, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid status error = %v, want ErrInvalidInput", err)
	}
}

func TestReviewActionsRequireValidVersionsAndForwardToStore(t *testing.T) {
	store := &fakeStore{value: Variant{ID: testVariantID, PostID: testPostID, Platform: domain.PlatformX}}
	service := NewService(store)
	if _, err := service.Approve(context.Background(), testVariantID, 7); err != nil {
		t.Fatal(err)
	}
	if store.approvedVersion != 7 {
		t.Fatalf("approval version = %d, want 7", store.approvedVersion)
	}
	if _, err := service.Reject(context.Background(), testVariantID, 8); err != nil {
		t.Fatal(err)
	}
	if store.rejectedVersion != 8 {
		t.Fatalf("rejection version = %d, want 8", store.rejectedVersion)
	}
	if _, err := service.Approve(context.Background(), testVariantID, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero version error = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateContentValidatesPlatformLimitsAndVersion(t *testing.T) {
	store := &fakeStore{value: Variant{ID: testVariantID, PostID: testPostID, Platform: domain.PlatformX}}
	service := NewService(store)
	if _, err := service.UpdateContent(context.Background(), testVariantID, 3, "Revised #social"); err != nil {
		t.Fatal(err)
	}
	if store.updatedVersion != 3 || store.updatedContent != "Revised #social" {
		t.Fatalf("update arguments = version %d, content %q", store.updatedVersion, store.updatedContent)
	}
	if _, err := service.UpdateContent(context.Background(), testVariantID, 3, string(make([]byte, 281))); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("invalid content error = %v, want ErrInvalidContent", err)
	}
	if store.updatedContent == string(make([]byte, 281)) {
		t.Fatal("store received overlength content")
	}
}
