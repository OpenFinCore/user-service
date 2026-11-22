package router

import (
	"fmt"
	"net/http"

	"ofc-user-service/handlers"
	"ofc-user-service/service"
)

// registerRegistrationRoutes mounts the end-user registration APIs. All of
// them are tenant scoped; each route lists the middleware it needs.
func registerRegistrationRoutes(mux *http.ServeMux, auth *service.AuthService, h *handlers.RegistrationHandler, urlPrefix string) {
	mux.Handle(fmt.Sprintf("POST %s/register-request", urlPrefix), chain(h.RequestOTP, auth.RequireTenant, auth.RequireDevice))
}
