// Package router mounts every API group. Each group lives in its own file
// and asks service.AuthService for the middleware its routes need.
package router

import (
	"net/http"

	"ofc-user-service/handlers"
	"ofc-user-service/service"
)

// New builds the service's HTTP handler from the wired container.
func New(container *service.Container) http.Handler {
	root := http.NewServeMux()

	// Liveness probe for orchestrators: outside the API guard, so it needs
	// no request id / timestamp headers.
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		service.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	root.Handle("/", newAPI(container.Auth, handlers.NewRegistrationHandler(container.Registration)))
	return root
}

// newAPI mounts every API group behind the global middleware
// (container.Auth.Common: request id + timestamp guard, access log, recover).
func newAPI(auth *service.AuthService, registration *handlers.RegistrationHandler) http.Handler {
	api := http.NewServeMux()

	registerRegistrationRoutes(api, auth, registration)

	api.Handle("/", auth.NotFound())
	return auth.Common(api)
}
