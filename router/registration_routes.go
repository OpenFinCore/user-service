package router

import (
	"fmt"
	"net/http"

	"ofc-user-service/handlers"
	"ofc-user-service/service"
)

// registerRegistrationRoutes mounts the end-user registration APIs. All of
// them are tenant scoped; each route lists the middleware it needs, in the
// order it runs. Rate limits go first by IP (before any other work), then
// by device once the device headers are validated.
func registerRegistrationRoutes(mux *http.ServeMux, auth *service.AuthService, h *handlers.RegistrationHandler, urlPrefix string) {
	mux.Handle(fmt.Sprintf("POST %s/register-request", urlPrefix), chain(h.RequestOTP,
		auth.RateLimit("register-request:ip", service.ByClientIP),
		auth.RequireTenant,
		auth.RequireDevice,
		auth.RateLimit("register-request:device", service.ByDevice),
	))
}
