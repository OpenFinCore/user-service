package models

import (
	"time"

	"github.com/google/uuid"
)

type RequestOTPRequest struct {
	PhoneNumber string `json:"phone_number"`
}

func (s *RequestOTPRequest) ValidatePhone() error {
	switch {
	case s.PhoneNumber == "":
		return NewAppError(CodePhoneNumberRequired)
	case !E164.MatchString(s.PhoneNumber):
		return NewAppError(CodePhoneNumberInvalid)
	}
	return nil
}

type RequestOTPResponse struct {
	ExpiresAt          time.Time `json:"expires_at"`
	ResendAfterSeconds int       `json:"resend_after_seconds"`
	// DeviceID is the devices row registered for this request (status
	// PENDING_VERIFICATION); the OTP is held against it.
	DeviceID uuid.UUID `json:"device_id"`
	// OTP is present only in the development stage.
	OTP *string `json:"otp,omitempty"`
}
