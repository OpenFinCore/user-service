package handlers

import (
	"net/http"

	"ofc-user-service/conf"
	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/service"
	"ofc-user-service/utils"
)

type registrationService interface {
	RequestOTP(ctx c.Context, in service.RequestOTPInput) (*service.RequestOTPResult, error)
	ConfirmRegistration(ctx c.Context, in service.ConfirmOtpInput) (*service.ConfirmOtpResponse, error)
}

type RegistrationHandler struct {
	registration registrationService
	config       *conf.Config
}

func NewRegistrationHandler(registration registrationService, config *conf.Config) *RegistrationHandler {
	return &RegistrationHandler{registration: registration, config: config}
}

func (h *RegistrationHandler) RequestOTP(w http.ResponseWriter, r *http.Request) {
	var req models.RequestOTPRequest
	if err := service.DecodeJSON(w, r, &req); err != nil {
		service.WriteError(w, r, err)
		return
	}

	req.PhoneNumber = utils.NormalizePhone(req.PhoneNumber)
	if err := req.ValidatePhone(); err != nil {
		service.WriteError(w, r, err)
		return
	}

	// Tenant and device reach the service on AppContext, set by the route
	// middleware (RequireTenant, RequireDevice).
	res, err := h.registration.RequestOTP(service.AppContext(r), service.RequestOTPInput{
		PhoneNumber: req.PhoneNumber,
		IPAddress:   service.ClientIP(r),
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		service.WriteError(w, r, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	service.WriteSuccess(w, http.StatusCreated, models.RequestOTPResponse{
		DeviceID:           res.DeviceID,
		ExpiresAt:          res.ExpiresAt,
		ResendAfterSeconds: int(res.ResendAfter.Seconds()),
		OTP:                res.Code,
	})
}

func (h *RegistrationHandler) ConfirmRegistration(w http.ResponseWriter, r *http.Request) {
	var req models.RequestOTPRequest
	if err := service.DecodeJSON(w, r, &req); err != nil {
		service.WriteError(w, r, err)
		return
	}

	req.PhoneNumber = utils.NormalizePhone(req.PhoneNumber)
	if err := req.ValidatePhoneAndOtp(h.config.OTP.Length); err != nil {
		service.WriteError(w, r, err)
		return
	}

	// Tenant and device reach the service on AppContext, set by the route
	// middleware (RequireTenant, RequireDevice).
	res, err := h.registration.ConfirmRegistration(service.AppContext(r), service.ConfirmOtpInput{
		PhoneNumber: req.PhoneNumber,
		OTP:         req.Otp,
		IPAddress:   service.ClientIP(r),
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		service.WriteError(w, r, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	service.WriteSuccess(w, http.StatusCreated, models.ConfirmOTPResponse{
		ExpiresAt: res.ExpiresAt,
		// Token:     res.Token,
	})
}
