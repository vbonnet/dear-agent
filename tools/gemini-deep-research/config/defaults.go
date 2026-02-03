package config

const (
	// DefaultOutputDir is the default output directory
	DefaultOutputDir = "./output"

	// DefaultCacheDir is the default cache directory for research results
	DefaultCacheDir = "~/src/ws/oss/research"

	// DefaultTimeout is the default Deep Research timeout in minutes
	DefaultTimeout = 60

	// DefaultPollInterval is the default status polling interval in seconds
	DefaultPollInterval = 10

	// DefaultRetryMax is the default maximum number of retries
	DefaultRetryMax = 3

	// DefaultRetryBackoff is the default initial retry backoff in seconds
	DefaultRetryBackoff = 1
)
