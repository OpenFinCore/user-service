package handlers

import (
	"net/http"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/service"
	"ofc-user-service/utils"
)

type registrationService interface {
	RequestOTP(ctx c.Context, in service.RequestOTPInput) (*service.RequestOTPResult, error)
}

type RegistrationHandler struct {
	registration registrationService
}

func NewRegistrationHandler(registration registrationService) *RegistrationHandler {
	return &RegistrationHandler{registration: registration}
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
	device := service.DeviceHeaders(r)
	if err := device.ValidateDevice(); err != nil {
		service.WriteError(w, r, err)
		return
	}

	res, err := h.registration.RequestOTP(service.AppContext(r), service.RequestOTPInput{
		PhoneNumber: req.PhoneNumber,
		Device:      device,
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
