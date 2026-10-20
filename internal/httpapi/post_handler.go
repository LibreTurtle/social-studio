package httpapi

import (
	"net/http"

	"github.com/LibreTurtle/social-studio/internal/post"
)

func (server *Server) createPost(writer http.ResponseWriter, request *http.Request) {
	if server.posts == nil {
		server.writeError(writer, request, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Post ingestion is unavailable", nil)
		return
	}
	var body post.CreateRequest
	if !server.decodeRequest(writer, request, &body) {
		return
	}
	created, err := server.posts.Create(request.Context(), body)
	if err != nil {
		server.handleError(writer, request, err)
		return
	}
	server.writeJSON(writer, request, http.StatusCreated, envelope{Success: true, Message: "Post created", Data: created})
}
