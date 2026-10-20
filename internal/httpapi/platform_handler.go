package httpapi

import (
	"net/http"
	"sort"

	"github.com/LibreTurtle/social-studio/internal/platform"
)

func (server *Server) listPlatforms(writer http.ResponseWriter, request *http.Request) {
	profiles := platform.DefaultProfiles()
	list := make([]platform.Profile, 0, len(profiles))
	for _, profile := range profiles {
		list = append(list, profile)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	server.writeJSON(writer, request, http.StatusOK, envelope{Success: true, Message: "Platform profiles retrieved", Data: list})
}
