package cliframe

import (
	"testing"
)

func TestExitCodeConstants(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"ExitSuccess", ExitSuccess, 0},
		{"ExitGeneralError", ExitGeneralError, 1},
		{"ExitMisuse", ExitMisuse, 2},
		{"ExitUsageError", ExitUsageError, 64},
		{"ExitDataError", ExitDataError, 65},
		{"ExitNoInput", ExitNoInput, 66},
		{"ExitServiceUnavailable", ExitServiceUnavailable, 69},
		{"ExitInternalError", ExitInternalError, 70},
		{"ExitTempFail", ExitTempFail, 75},
		{"ExitPermissionDenied", ExitPermissionDenied, 77},
		{"ExitConfig", ExitConfig, 78},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.code, tt.expected)
			}
		})
	}
}

func TestGetExitCodeInfo_KnownCode(t *testing.T) {
	info := GetExitCodeInfo(ExitServiceUnavailable)

	if info == nil {
		t.Fatal("Expected non-nil info")
	}

	if info.Code != ExitServiceUnavailable {
		t.Errorf("Expected code %d, got %d", ExitServiceUnavailable, info.Code)
	}

	if info.Name != "ServiceUnavailable" {
		t.Errorf("Expected name 'ServiceUnavailable', got %s", info.Name)
	}

	if !info.Retryable {
		t.Error("Expected ServiceUnavailable to be retryable")
	}

	if info.Category != "temporary" {
		t.Errorf("Expected category 'temporary', got %s", info.Category)
	}
}

func TestGetExitCodeInfo_UnknownCode(t *testing.T) {
	info := GetExitCodeInfo(999)

	if info == nil {
		t.Fatal("Expected non-nil info for unknown code")
	}

	if info.Code != 999 {
		t.Errorf("Expected code 999, got %d", info.Code)
	}

	if info.Name != "Unknown" {
		t.Errorf("Expected name 'Unknown', got %s", info.Name)
	}

	if info.Retryable {
		t.Error("Expected unknown code to not be retryable")
	}

	if info.Category != "unknown" {
		t.Errorf("Expected category 'unknown', got %s", info.Category)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected bool
	}{
		{"ServiceUnavailable", ExitServiceUnavailable, true},
		{"TempFail", ExitTempFail, true},
		{"GeneralError", ExitGeneralError, false},
		{"UsageError", ExitUsageError, false},
		{"PermissionDenied", ExitPermissionDenied, false},
		{"Success", ExitSuccess, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.code)
			if result != tt.expected {
				t.Errorf("IsRetryable(%d) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestExitCodes(t *testing.T) {
	codes := ExitCodes()

	if len(codes) == 0 {
		t.Error("Expected non-empty exit codes list")
	}

	// Check that all standard codes are present
	found := make(map[int]bool)
	for _, info := range codes {
		found[info.Code] = true
	}

	expectedCodes := []int{
		ExitSuccess,
		ExitGeneralError,
		ExitMisuse,
		ExitUsageError,
		ExitDataError,
		ExitNoInput,
		ExitServiceUnavailable,
		ExitInternalError,
		ExitTempFail,
		ExitPermissionDenied,
		ExitConfig,
	}

	for _, code := range expectedCodes {
		if !found[code] {
			t.Errorf("Expected code %d to be in exit codes list", code)
		}
	}
}

func TestHTTPStatusToExitCode(t *testing.T) {
	tests := []struct {
		name       string
		httpStatus int
		expected   int
	}{
		{"200 OK", 200, ExitSuccess},
		{"201 Created", 201, ExitSuccess},
		{"299 Success range", 299, ExitSuccess},
		{"400 Bad Request", 400, ExitUsageError},
		{"401 Unauthorized", 401, ExitPermissionDenied},
		{"403 Forbidden", 403, ExitPermissionDenied},
		{"404 Not Found", 404, ExitNoInput},
		{"408 Request Timeout", 408, ExitTempFail},
		{"429 Too Many Requests", 429, ExitTempFail},
		{"500 Internal Server Error", 500, ExitInternalError},
		{"503 Service Unavailable", 503, ExitServiceUnavailable},
		{"504 Gateway Timeout", 504, ExitTempFail},
		{"418 I'm a teapot", 418, ExitGeneralError}, // Unknown status
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HTTPStatusToExitCode(tt.httpStatus)
			if result != tt.expected {
				t.Errorf("HTTPStatusToExitCode(%d) = %d, want %d",
					tt.httpStatus, result, tt.expected)
			}
		})
	}
}

func TestExitCodeInfo_Categories(t *testing.T) {
	tests := []struct {
		code     int
		category string
	}{
		{ExitSuccess, "success"},
		{ExitGeneralError, "user_error"},
		{ExitUsageError, "user_error"},
		{ExitInternalError, "system_error"},
		{ExitServiceUnavailable, "temporary"},
		{ExitTempFail, "temporary"},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			info := GetExitCodeInfo(tt.code)
			if info.Category != tt.category {
				t.Errorf("Expected category %s for code %d, got %s",
					tt.category, tt.code, info.Category)
			}
		})
	}
}

func TestExitCodeInfo_AllFieldsPopulated(t *testing.T) {
	// Verify that all registered exit codes have complete metadata
	codes := ExitCodes()

	for _, info := range codes {
		if info.Code == 0 && info.Name == "" {
			t.Error("Found exit code with missing metadata")
		}

		if info.Name == "" {
			t.Errorf("Code %d missing Name", info.Code)
		}

		if info.Description == "" {
			t.Errorf("Code %d missing Description", info.Code)
		}

		if info.Category == "" {
			t.Errorf("Code %d missing Category", info.Code)
		}

		// Category should be one of the expected values
		validCategories := map[string]bool{
			"success":      true,
			"user_error":   true,
			"system_error": true,
			"temporary":    true,
		}

		if !validCategories[info.Category] {
			t.Errorf("Code %d has invalid category: %s", info.Code, info.Category)
		}
	}
}

func TestHTTPStatusToExitCode_SuccessRange(t *testing.T) {
	// Test all 2xx success codes
	for status := 200; status < 300; status++ {
		result := HTTPStatusToExitCode(status)
		if result != ExitSuccess {
			t.Errorf("HTTP %d should map to ExitSuccess, got %d", status, result)
		}
	}
}

func TestIsRetryable_OnlyTemporaryErrors(t *testing.T) {
	// Test that only temporary errors are retryable
	codes := ExitCodes()

	for _, info := range codes {
		if info.Category == "temporary" && !info.Retryable {
			t.Errorf("Code %d is in 'temporary' category but not marked retryable", info.Code)
		}

		if info.Retryable && info.Category != "temporary" {
			t.Errorf("Code %d is retryable but not in 'temporary' category", info.Code)
		}
	}
}
