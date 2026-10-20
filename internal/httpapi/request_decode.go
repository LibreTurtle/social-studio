package httpapi

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"net/http"
)

const maxRequestBodyBytes = 1 << 20

func (server *Server) decodeRequest(writer http.ResponseWriter, request *http.Request, destination any) bool {
	data, err := io.ReadAll(io.LimitReader(request.Body, maxRequestBodyBytes+1))
	if err != nil {
		server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Request body could not be read", nil)
		return false
	}
	if len(data) > maxRequestBodyBytes {
		server.writeError(writer, request, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body exceeds the 1 MiB limit", nil)
		return false
	}
	if err := json.Unmarshal(data, destination, json.RejectUnknownMembers(true)); err != nil || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		server.writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid", nil)
		return false
	}
	return true
}
