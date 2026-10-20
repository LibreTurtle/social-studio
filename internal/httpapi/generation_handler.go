package httpapi

import (
	"net/http"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/generation"
	"github.com/LibreTurtle/social-studio/internal/platform"
)

type generateVariantsRequest struct {
	PostID      string                     `json:"post_id"`
	Preferences []variantPreferenceRequest `json:"preferences"`
}

type variantPreferenceRequest struct {
	Platform     domain.Platform `json:"platform"`
	Tone         domain.Tone     `json:"tone"`
	TargetLength *int            `json:"target_length"`
	HashtagCount *int            `json:"hashtag_count"`
	Instructions string          `json:"instructions,omitempty"`
}

func (body generateVariantsRequest) generationRequest() generation.Request {
	profiles := platform.DefaultProfiles()
	preferences := make([]generation.Preference, 0, len(body.Preferences))
	for _, preference := range body.Preferences {
		targetLength, hashtagCount := 0, 0
		if profile, ok := profiles[preference.Platform]; ok {
			targetLength = profile.MaxLength
		}
		if preference.TargetLength != nil {
			targetLength = *preference.TargetLength
		}
		if preference.HashtagCount != nil {
			hashtagCount = *preference.HashtagCount
		}
		preferences = append(preferences, generation.Preference{
			Platform: preference.Platform, Tone: preference.Tone,
			TargetLength: targetLength, HashtagCount: hashtagCount,
			Instructions: preference.Instructions,
		})
	}
	return generation.Request{PostID: body.PostID, Preferences: preferences}
}

func (server *Server) generateVariants(writer http.ResponseWriter, request *http.Request) {
	if server.generations == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "AI generation is unavailable", nil)
		return
	}
	var body generateVariantsRequest
	if !server.decodeRequest(writer, request, &body) {
		return
	}
	result, err := server.generations.Generate(request.Context(), body.generationRequest())
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusCreated, envelope{Success: true, Message: "Variants generated", Data: result})
}

func (server *Server) getGeneration(writer http.ResponseWriter, request *http.Request) {
	if server.generations == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Generation history is unavailable", nil)
		return
	}
	result, err := server.generations.Get(request.PathValue("sessionID"))
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Generation retrieved", Data: result})
}

func (server *Server) getCandidates(writer http.ResponseWriter, request *http.Request) {
	if server.generations == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Generation history is unavailable", nil)
		return
	}
	result, err := server.generations.Candidates(request.PathValue("sessionID"))
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Candidates retrieved", Data: result})
}

func (server *Server) selectCandidate(writer http.ResponseWriter, request *http.Request) {
	if server.generations == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Generation history is unavailable", nil)
		return
	}
	var body struct {
		CandidateID string `json:"candidate_id"`
	}
	if !server.decodeRequest(writer, request, &body) {
		return
	}
	result, err := server.generations.Select(request.PathValue("sessionID"), body.CandidateID)
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Candidate selected", Data: result})
}
