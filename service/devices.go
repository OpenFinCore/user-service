package service

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	c "ofc-user-service/context"
	"ofc-user-service/models"
	"ofc-user-service/repository"
)

type DeviceService struct {
	*CRUDService[models.Device]
	statuses *DeviceStatusService
}

func NewDeviceService(store repository.Store, statuses *DeviceStatusService) *DeviceService {
	return &DeviceService{CRUDService: NewCRUDService[models.Device](store), statuses: statuses}
}

func (s *DeviceService) GetDevice(
	ctx c.Context,
	device *models.DeviceFilters,
) (*models.Device, error) {

	db := repository.DBFrom(ctx, s.store)

	if device == nil {
		return nil, nil
	}

	filters := []repository.Filter{}

	if device.DeviceID != "" {
		filters = append(filters,
			repository.Eq("device_uuid", device.DeviceID),
		)
	}

	if device.DeviceName != "" {
		filters = append(filters,
			repository.Eq("device_name", device.DeviceName),
		)
	}

	if device.DeviceType != "" {
		filters = append(filters,
			repository.Eq("device_type", device.DeviceType),
		)
	}

	if device.OSName != "" {
		filters = append(filters,
			repository.Eq("os_name", device.OSName),
		)
	}

	if device.OSVersion != "" {
		filters = append(filters,
			repository.Eq("os_version", device.OSVersion),
		)
	}

	if device.AppVersion != "" {
		filters = append(filters,
			repository.Eq("app_version", device.AppVersion),
		)
	}

	result, err := repository.Get[models.Device](
		db,
		repository.QueryOptions{
			Filters: filters,
		},
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// upsertPendingDeviceSQL inserts a pending device or refreshes the existing
// pending row for the same tenant + device uuid, in one atomic statement.
// The conflict target is the partial unique index uq_devices_pending_device
// (service_provider_id, device_uuid) WHERE user_id IS NULL, so concurrent
// requests from one device can never create duplicate rows. A pending row
// in any other status (BLOCKED, REVOKED) is left alone and no id is
// returned: such a device must not be revived by asking for a new code.
const upsertPendingDeviceSQL = `
INSERT INTO devices (
    service_provider_id, user_id, device_uuid, device_name, device_type,
    os_name, os_version, app_version, ip_address, user_agent,
    is_trusted, status_id, last_seen_at
) VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?::inet, ?, FALSE, ?, ?)
ON CONFLICT (service_provider_id, device_uuid) WHERE user_id IS NULL
DO UPDATE SET
    device_name  = EXCLUDED.device_name,
    device_type  = EXCLUDED.device_type,
    os_name      = EXCLUDED.os_name,
    os_version   = EXCLUDED.os_version,
    app_version  = EXCLUDED.app_version,
    ip_address   = EXCLUDED.ip_address,
    user_agent   = EXCLUDED.user_agent,
    last_seen_at = EXCLUDED.last_seen_at
WHERE devices.status_id = EXCLUDED.status_id
RETURNING id`

// RegisterPending registers the device a not-yet-existing user is signing
// up from: a devices row with no user and the PENDING_VERIFICATION status.
// A pending row for the same tenant + device uuid is reused (refreshed with
// the latest details) so repeated OTP requests don't pile up rows. Must run
// inside RunInTx: devices is tenant-scoped by row-level security.
//
// It returns CodeDeviceNotAllowed when the device's pending row exists but
// is no longer PENDING_VERIFICATION (blocked or revoked).
func (s *DeviceService) RegisterPending(ctx c.Context, spID uuid.UUID, info models.Device) (*models.Device, error) {
	if info.DeviceUUID == "" {
		return nil, models.ErrDeviceUUIDRequired
	}
	if info.DeviceType == "" {
		return nil, models.ErrDeviceTypeRequired
	}
	pendingID, err := s.statuses.IDByName(ctx, models.DeviceStatusPendingVerification)
	if err != nil {
		return nil, fmt.Errorf("device: resolving status: %w", err)
	}

	now := time.Now().UTC()
	var ip *string
	if info.IPAddress != nil {
		v := info.IPAddress.String()
		ip = &v
	}

	var ids []uuid.UUID
	err = repository.DBFrom(ctx, s.store).Raw(upsertPendingDeviceSQL,
		spID, info.DeviceUUID, info.DeviceName, info.DeviceType,
		info.OSName, info.OSVersion, info.AppVersion, ip, info.UserAgent,
		pendingID, now,
	).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("device: registering pending device: %w", err)
	}
	if len(ids) == 0 {
		return nil, models.NewAppError(models.CodeDeviceNotAllowed)
	}

	info.ID = ids[0]
	info.ServiceProviderID = spID
	info.UserID = nil
	info.StatusID = pendingID
	info.LastSeenAt = &now
	return &info, nil
}
