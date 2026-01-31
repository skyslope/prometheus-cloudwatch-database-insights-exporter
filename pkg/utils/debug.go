package utils

var (
	debugEnabled bool
)

// SetDebugEnabled sets the debug logging flag.
// This should be called once at application startup based on the -debug flag.
func SetDebugEnabled(enabled bool) {
	debugEnabled = enabled
}

// IsDebugEnabled checks if debug logging is enabled.
func IsDebugEnabled() bool {
	return debugEnabled
}
