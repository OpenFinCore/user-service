package service

import (
	"testing"

	"ofc-user-service/models"
)

// catalog lists every response code; add new codes here too.
var catalog = []models.ResponseCode{
	models.CodeSuccess,
	models.CodeServiceProviderHeaderRequired, models.CodeServiceProviderHeaderInvalid,
	models.CodeRequestIDHeaderRequired, models.CodeRequestIDHeaderInvalid,
	models.CodeTimestampHeaderRequired, models.CodeTimestampHeaderInvalid, models.CodeTimestampOutOfWindow,
	models.CodeDuplicateRequest, models.CodeInvalidRequestBody, models.CodeRequestBodyTooLarge, models.CodeRouteNotFound,
	models.CodeDeviceIDHeaderRequired, models.CodeDeviceIDHeaderInvalid, models.CodeDeviceTypeHeaderRequired,
	models.CodeDeviceTypeHeaderInvalid, models.CodeDeviceHeaderTooLong,
	models.CodePhoneNumberRequired, models.CodePhoneNumberInvalid,
	models.CodeServiceProviderNotFound, models.CodePhoneAlreadyRegistered,
	models.CodeOTPResendCooldown, models.CodeOTPHourlyLimit, models.CodeOTPDeliveryFailed,
	models.CodeInternal, models.CodeServiceUnavailable,
}

func TestResponseCodes_Unique(t *testing.T) {
	seen := map[int]string{}
	for _, rc := range catalog {
		if prev, ok := seen[rc.Code]; ok {
			t.Fatalf("code %d used by both %q and %q", rc.Code, prev, rc.Message)
		}
		seen[rc.Code] = rc.Message
	}
	if models.CodeSuccess.Code != 0 {
		t.Fatalf("success must be code 0, got %d", models.CodeSuccess.Code)
	}
}

func TestResponseCodes_ErrorsHaveKnownKind(t *testing.T) {
	for _, rc := range catalog[1:] {
		if _, ok := httpStatusByKind[rc.Kind]; !ok {
			t.Fatalf("code %d has no HTTP status for kind %q", rc.Code, rc.Kind)
		}
	}
}
