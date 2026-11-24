package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/repository"
	utils "ofc-user-service/utils"
)

type UserSessionService struct {
	*CRUDService[models.UserSession]
}

func NewUserSessionService(store repository.Store) *UserSessionService {
	return &UserSessionService{NewCRUDService[models.UserSession](store)}
}

type RefreshTokenService struct {
	*CRUDService[models.RefreshToken]
}

func NewRefreshTokenService(store repository.Store) *RefreshTokenService {
	return &RefreshTokenService{NewCRUDService[models.RefreshToken](store)}
}

type PasswordResetTokenService struct {
	*CRUDService[models.PasswordResetToken]
}

func NewPasswordResetTokenService(store repository.Store) *PasswordResetTokenService {
	return &PasswordResetTokenService{NewCRUDService[models.PasswordResetToken](store)}
}

type AuditLogService struct{ *CRUDService[models.AuditLog] }

func NewAuditLogService(store repository.Store) *AuditLogService {
	return &AuditLogService{NewCRUDService[models.AuditLog](store)}
}

// AuditEvent is what a domain service reports to RecordEvent. Payload must
// not contain secrets or unmasked personal data.
type AuditEvent struct {
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Payload      map[string]any
	IPAddress    string
	UserAgent    string
}

// RecordEvent is the one way domain services write audit entries: it
// encodes the payload, parses the IP, trims the user agent and records the
// entry for the tenant and actor carried by ctx.
func (s *AuditLogService) RecordEvent(ctx c.Context, ev AuditEvent) error {
	entry := models.AuditLog{
		Action:       ev.Action,
		ResourceType: ev.ResourceType,
		ResourceID:   ev.ResourceID,
		IPAddress:    utils.ParseIP(ev.IPAddress),
		UserAgent:    utils.OptionalString(utils.Truncate(strings.TrimSpace(ev.UserAgent), 512)),
	}
	if len(ev.Payload) > 0 {
		payload, err := json.Marshal(ev.Payload)
		if err != nil {
			return err
		}
		entry.Payload = datatypes.JSON(payload)
	}
	return s.Record(ctx, entry)
}

// Record appends one audit entry. Tenant and actor default to the ones
// carried by ctx; an empty payload is stored as {}. audit_logs is
// append-only: entries are never updated or deleted.
func (s *AuditLogService) Record(ctx c.Context, entry models.AuditLog) error {
	if entry.ServiceProviderID == uuid.Nil {
		if sp := ctx.ServiceProviderID(); sp != nil {
			entry.ServiceProviderID = *sp
		}
	}
	if entry.ServiceProviderID == uuid.Nil {
		return errors.New("audit: service provider id is required")
	}
	if entry.ActorID == nil {
		entry.ActorID = ctx.ActorID()
	}
	if len(entry.Payload) == 0 {
		entry.Payload = datatypes.JSON("{}")
	}
	return repository.RunInTx(ctx, s.store, func(ctx c.Context) error {
		if err := s.Create(ctx, &entry); err != nil {
			return fmt.Errorf("audit: writing %s: %w", entry.Action, err)
		}
		return nil
	})
}
