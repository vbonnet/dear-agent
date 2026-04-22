package cliframe

// Standard exit codes (sysexits.h compatible)
const (
	// ExitSuccess indicates successful execution
	ExitSuccess = 0

	// ExitGeneralError indicates a general application error
	ExitGeneralError = 1

	// ExitMisuse indicates command misuse (bad arguments)
	ExitMisuse = 2

	// ExitUsageError indicates command line usage error
	ExitUsageError = 64

	// ExitDataError indicates data format error
	ExitDataError = 65

	// ExitNoInput indicates cannot open input
	ExitNoInput = 66

	// ExitNoUser indicates addressee unknown
	ExitNoUser = 67

	// ExitNoHost indicates host name unknown
	ExitNoHost = 68

	// ExitServiceUnavailable indicates service unavailable (retry recommended)
	ExitServiceUnavailable = 69

	// ExitInternalError indicates internal software error
	ExitInternalError = 70

	// ExitOSError indicates system error (e.g., can't fork)
	ExitOSError = 71

	// ExitOSFile indicates critical OS file missing
	ExitOSFile = 72

	// ExitCantCreate indicates can't create output file
	ExitCantCreate = 73

	// ExitIOError indicates input/output error
	ExitIOError = 74

	// ExitTempFail indicates temporary failure (retry recommended)
	ExitTempFail = 75

	// ExitProtocol indicates remote error in protocol
	ExitProtocol = 76

	// ExitPermissionDenied indicates permission denied
	ExitPermissionDenied = 77

	// ExitConfig indicates configuration error
	ExitConfig = 78
)

// ExitCodeInfo provides metadata about exit codes
type ExitCodeInfo struct {
	Code        int
	Name        string
	Description string
	Retryable   bool
	Category    string // "user_error", "system_error", "temporary"
}

// exitCodeRegistry maps exit codes to their metadata
var exitCodeRegistry = map[int]ExitCodeInfo{
	ExitSuccess: {
		Code:        ExitSuccess,
		Name:        "Success",
		Description: "Successful execution",
		Retryable:   false,
		Category:    "success",
	},
	ExitGeneralError: {
		Code:        ExitGeneralError,
		Name:        "GeneralError",
		Description: "General application error",
		Retryable:   false,
		Category:    "user_error",
	},
	ExitMisuse: {
		Code:        ExitMisuse,
		Name:        "Misuse",
		Description: "Command misuse (bad arguments)",
		Retryable:   false,
		Category:    "user_error",
	},
	ExitUsageError: {
		Code:        ExitUsageError,
		Name:        "UsageError",
		Description: "Command line usage error",
		Retryable:   false,
		Category:    "user_error",
	},
	ExitDataError: {
		Code:        ExitDataError,
		Name:        "DataError",
		Description: "Data format error",
		Retryable:   false,
		Category:    "user_error",
	},
	ExitNoInput: {
		Code:        ExitNoInput,
		Name:        "NoInput",
		Description: "Cannot open input",
		Retryable:   false,
		Category:    "user_error",
	},
	ExitServiceUnavailable: {
		Code:        ExitServiceUnavailable,
		Name:        "ServiceUnavailable",
		Description: "Service unavailable (retry recommended)",
		Retryable:   true,
		Category:    "temporary",
	},
	ExitInternalError: {
		Code:        ExitInternalError,
		Name:        "InternalError",
		Description: "Internal software error",
		Retryable:   false,
		Category:    "system_error",
	},
	ExitTempFail: {
		Code:        ExitTempFail,
		Name:        "TempFail",
		Description: "Temporary failure (retry recommended)",
		Retryable:   true,
		Category:    "temporary",
	},
	ExitPermissionDenied: {
		Code:        ExitPermissionDenied,
		Name:        "PermissionDenied",
		Description: "Permission denied",
		Retryable:   false,
		Category:    "user_error",
	},
	ExitConfig: {
		Code:        ExitConfig,
		Name:        "ConfigError",
		Description: "Configuration error",
		Retryable:   false,
		Category:    "user_error",
	},
}

// GetExitCodeInfo returns metadata for an exit code
func GetExitCodeInfo(code int) *ExitCodeInfo {
	if info, ok := exitCodeRegistry[code]; ok {
		return &info
	}
	// Return generic info for unknown codes
	return &ExitCodeInfo{
		Code:        code,
		Name:        "Unknown",
		Description: "Unknown exit code",
		Retryable:   false,
		Category:    "unknown",
	}
}

// IsRetryable returns true if exit code indicates retriable error
func IsRetryable(code int) bool {
	info := GetExitCodeInfo(code)
	return info.Retryable
}

// ExitCodes returns all defined exit codes with descriptions
func ExitCodes() []ExitCodeInfo {
	codes := make([]ExitCodeInfo, 0, len(exitCodeRegistry))
	for _, info := range exitCodeRegistry {
		codes = append(codes, info)
	}
	return codes
}

// HTTPStatusToExitCode maps HTTP status codes to exit codes
func HTTPStatusToExitCode(status int) int {
	switch {
	case status >= 200 && status < 300:
		return ExitSuccess
	case status == 400:
		return ExitUsageError
	case status == 401, status == 403:
		return ExitPermissionDenied
	case status == 404:
		return ExitNoInput
	case status == 408, status == 504:
		return ExitTempFail
	case status == 429:
		return ExitTempFail // Rate limit
	case status == 500:
		return ExitInternalError
	case status == 503:
		return ExitServiceUnavailable
	default:
		return ExitGeneralError
	}
}
