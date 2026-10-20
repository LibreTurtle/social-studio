package httpapi

import (
	"net/http"
)

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Service is healthy", Data: map[string]string{"status": "ok"}})
}
