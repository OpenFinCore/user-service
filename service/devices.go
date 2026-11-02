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

// RegisterPending registers the device a not-yet-existing user is signing
// up from: a devices row with no user and the PENDING_VERIFICATION status.
// A pending row for the same tenant + device uuid is reused (refreshed with
// the latest details) so repeated OTP requests don't pile up rows. Must run
// inside RunInTx: devices is tenant-scoped by row-level security.
func (s *DeviceService) RegisterPending(ctx c.Context, spID uuid.UUID, info models.Device) (*models.Device, error) {
	pendingID, err := s.statuses.IDByName(ctx, models.DeviceStatusPendingVerification)
	if err != nil {
		return nil, fmt.Errorf("device: resolving status: %w", err)
	}

	now := time.Now().UTC()
	info.ServiceProviderID = spID
	info.UserID = nil
	info.LastSeenAt = &now

	// Only the id is read back: the inet columns don't scan into *net.IP.
	var ids []uuid.UUID
	err = repository.DBFrom(ctx, s.store).Model(&models.Device{}).
		Where("service_provider_id = ? AND device_uuid = ? AND user_id IS NULL AND status_id = ?", spID, info.DeviceUUID, pendingID).
		Limit(1).Pluck("id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("device: loading pending device: %w", err)
	}

	if len(ids) > 0 {
		info.ID = ids[0]
		info.StatusID = pendingID
		if _, err := s.Update(ctx, repository.QueryOptions{Filters: []repository.Filter{repository.Eq("id", info.ID)}}, map[string]interface{}{
			"device_name":  info.DeviceName,
			"device_type":  info.DeviceType,
			"os_name":      info.OSName,
			"os_version":   info.OSVersion,
			"app_version":  info.AppVersion,
			"ip_address":   info.IPAddress,
			"user_agent":   info.UserAgent,
			"last_seen_at": info.LastSeenAt,
		}); err != nil {
			return nil, fmt.Errorf("device: updating pending device: %w", err)
		}
		return &info, nil
	}

	device, err := info.NewDevice(pendingID)
	if err != nil {
		return nil, err
	}
	if err := s.Create(ctx, device); err != nil {
		return nil, fmt.Errorf("device: creating pending device: %w", err)
	}
	return device, nil
}
