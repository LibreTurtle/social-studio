package generation

import (
	"sync"
	"time"
	"uuid"
)

type sessionEntry struct{ result Result }
type SessionCache struct {
	ttl     time.Duration
	maximum int
	mutex   sync.Mutex
	entries map[string]sessionEntry
}

func NewSessionCache(ttl time.Duration, maximum int) *SessionCache {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if maximum <= 0 {
		maximum = 10000
	}
	return &SessionCache{ttl: ttl, maximum: maximum, entries: make(map[string]sessionEntry)}
}

func (cache *SessionCache) TTL() time.Duration { return cache.ttl }

func (cache *SessionCache) Put(result Result) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	now := time.Now()
	for id, entry := range cache.entries {
		if !now.Before(entry.result.ExpiresAt) {
			delete(cache.entries, id)
		}
	}
	if len(cache.entries) >= cache.maximum {
		cache.removeOldest()
	}
	cache.entries[result.ID] = sessionEntry{result: result}
}

func (cache *SessionCache) Get(id string) (Result, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	entry, exists := cache.entries[id]
	if !exists || !time.Now().Before(entry.result.ExpiresAt) {
		delete(cache.entries, id)
		return Result{}, ErrSessionNotFound
	}
	return entry.result, nil
}

func (cache *SessionCache) removeOldest() {
	var oldestID string
	var oldestTime time.Time
	for id, entry := range cache.entries {
		if oldestID == "" || entry.result.CreatedAt.Before(oldestTime) {
			oldestID, oldestTime = id, entry.result.CreatedAt
		}
	}
	delete(cache.entries, oldestID)
}

func newResult(postID string, ttl time.Duration, capacity int) Result {
	created := time.Now().UTC()
	return Result{ID: newID(), PostID: postID, CreatedAt: created, ExpiresAt: created.Add(ttl), Candidates: make([]Candidate, 0, capacity)}
}

func newID() string { return uuid.NewV4().String() }

func (service *Service) Get(id string) (Result, error) { return service.sessions.Get(id) }
func (service *Service) Candidates(id string) ([]Candidate, error) {
	result, err := service.Get(id)
	if err != nil {
		return nil, err
	}
	return result.Candidates, nil
}
func (service *Service) Select(sessionID, candidateID string) (Candidate, error) {
	result, err := service.Get(sessionID)
	if err != nil {
		return Candidate{}, err
	}
	for _, candidate := range result.Candidates {
		if candidate.ID == candidateID {
			return candidate, nil
		}
	}
	return Candidate{}, ErrSessionNotFound
}
