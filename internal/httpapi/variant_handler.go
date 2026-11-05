package httpapi

import (
	"net/http"
	"strconv"

	"github.com/LibreTurtle/social-studio/internal/domain"
	"github.com/LibreTurtle/social-studio/internal/variant"
)

type variantEditRequest struct {
	Version int64  `json:"version"`
	Content string `json:"content"`
}

type variantReviewRequest struct {
	Version int64 `json:"version"`
}

func (server *Server) listVariants(writer http.ResponseWriter, request *http.Request) {
	if server.variants == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Variant review is unavailable", nil)
		return
	}
	limit, err := queryInteger(request, "limit", variant.DefaultPageSize)
	if err != nil {
		server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Pagination parameters are invalid", nil)
		return
	}
	offset, err := queryInteger(request, "offset", 0)
	if err != nil {
		server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Pagination parameters are invalid", nil)
		return
	}
	variants, err := server.variants.ListByPost(request.Context(), request.PathValue("postID"), domain.VariantStatus(request.URL.Query().Get("status")), limit, offset)
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Variants retrieved", Data: variants})
}

func (server *Server) getVariant(writer http.ResponseWriter, request *http.Request) {
	if server.variants == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Variant review is unavailable", nil)
		return
	}
	value, err := server.variants.Get(request.Context(), request.PathValue("variantID"))
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Variant retrieved", Data: value})
}

func (server *Server) editVariant(writer http.ResponseWriter, request *http.Request) {
	if server.variants == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Variant review is unavailable", nil)
		return
	}
	var body variantEditRequest
	if !server.decodeRequest(writer, request, &body) {
		return
	}
	value, err := server.variants.UpdateContent(request.Context(), request.PathValue("variantID"), body.Version, body.Content)
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Variant updated", Data: value})
}

func (server *Server) approveVariant(writer http.ResponseWriter, request *http.Request) {
	server.reviewVariant(writer, request, true)
}

func (server *Server) rejectVariant(writer http.ResponseWriter, request *http.Request) {
	server.reviewVariant(writer, request, false)
}

func (server *Server) reviewVariant(writer http.ResponseWriter, request *http.Request, approve bool) {
	if server.variants == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Variant review is unavailable", nil)
		return
	}
	var body variantReviewRequest
	if !server.decodeRequest(writer, request, &body) {
		return
	}
	var value variant.Variant
	var err error
	if approve {
		value, err = server.variants.Approve(request.Context(), request.PathValue("variantID"), body.Version)
	} else {
		value, err = server.variants.Reject(request.Context(), request.PathValue("variantID"), body.Version)
	}
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	message := "Variant rejected"
	if approve {
		message = "Variant approved"
	}
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: message, Data: value})
}

func queryInteger(request *http.Request, key string, defaultValue int) (int, error) {
	value := request.URL.Query().Get(key)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}
