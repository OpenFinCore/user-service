package service

import (
	"errors"

	c "ofc-user-service/context"
)

// SMSSender delivers a text message to an E.164 phone number.
type SMSSender interface {
	Send(ctx c.Context, to, message string) error
}

// ErrSMSNotConfigured is returned when no SMS provider is wired up.
var ErrSMSNotConfigured = errors.New("sms provider is not configured")

// UnconfiguredSMSSender is the placeholder until a real provider is
// integrated. It always fails, so a non-development deployment can never
// silently "send" codes nowhere.
type UnconfiguredSMSSender struct{}

func (UnconfiguredSMSSender) Send(c.Context, string, string) error { return ErrSMSNotConfigured }
