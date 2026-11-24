// Package router mounts every API group. Each group lives in its own file
// and asks service.AuthService for the middleware its routes need.
package router

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ofc-user-service/handlers"
	"ofc-user-service/models"
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

	// Readiness probe: only route traffic here while Postgres and Redis
	// answer. Failures are logged, never returned (they may name hosts).
	root.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := container.Ready(ctx); err != nil {
			service.WriteError(w, r, models.NewAppError(models.CodeServiceUnavailable).WithCause(err))
			return
		}
		service.WriteSuccess(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	root.Handle("/", newAPI(container.Auth, handlers.NewRegistrationHandler(container.Registration)))
	return root
}

// newAPI mounts every API group behind the global middleware
// (container.Auth.Common: request id + timestamp guard, access log, recover).
func newAPI(auth *service.AuthService, registration *handlers.RegistrationHandler) http.Handler {
	api := http.NewServeMux()
	urlPrefixV1 := fmt.Sprintf("/%s/%s", models.Version1, models.ServiceUrlPrefix)

	registerRegistrationRoutes(api, auth, registration, urlPrefixV1)

	api.Handle("/", auth.NotFound())
	return auth.Common(api)
}

// chain wraps h in route-level middleware, applied in the order listed:
// chain(h, a, b) runs a, then b, then h.
func chain(h http.HandlerFunc, mws ...func(http.Handler) http.Handler) http.Handler {
	var out http.Handler = h
	for i := len(mws) - 1; i >= 0; i-- {
		out = mws[i](out)
	}
	return out
}
