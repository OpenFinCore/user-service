package router

import (
	"net/http"

	"ofc-user-service/handlers"
	"ofc-user-service/service"
)

// registerRegistrationRoutes mounts the end-user registration APIs. All of
// them are tenant scoped.
func registerRegistrationRoutes(mux *http.ServeMux, auth *service.AuthService, h *handlers.RegistrationHandler) {
	mux.Handle("POST /v1/users/register/request", auth.RequireTenant(http.HandlerFunc(h.RequestOTP)))
}
