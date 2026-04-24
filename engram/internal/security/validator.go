package security

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// NetworkPermissionType represents the type of network permission
type NetworkPermissionType string

const (
	// PermTypeWildcard represents unrestricted network access ("*")
	PermTypeWildcard NetworkPermissionType = "wildcard"

	// PermTypeDomain represents a domain name (e.g., "github.com")
	PermTypeDomain NetworkPermissionType = "domain"

	// PermTypeIPv4 represents an IPv4 address (e.g., "1.1.1.1")
	PermTypeIPv4 NetworkPermissionType = "ipv4"

	// PermTypeIPv6 represents an IPv6 address (e.g., "2001:db8::1")
	PermTypeIPv6 NetworkPermissionType = "ipv6"

	// PermTypeCIDR represents a CIDR range (e.g., "192.168.1.0/24")
	PermTypeCIDR NetworkPermissionType = "cidr"
)

// Validator validates plugin permissions and configurations
type Validator struct {
	logger *slog.Logger
}

// NewValidator creates a new security validator
func NewValidator() *Validator {
	return &Validator{
		logger: slog.New(slog.NewJSONHandler(os.Stderr, nil)),
	}
}

// NewValidatorWithLogger creates a new security validator with a custom logger
func NewValidatorWithLogger(logger *slog.Logger) *Validator {
	return &Validator{
		logger: logger,
	}
}

// ValidatePermissions validates that requested permissions are reasonable
func (v *Validator) ValidatePermissions(permissions Permissions) error {
	// Validate filesystem paths
	for _, path := range permissions.Filesystem {
		if err := v.validateFilesystemPath(path); err != nil {
			return fmt.Errorf("invalid filesystem permission %q: %w", path, err)
		}
	}

	// Validate network permissions
	for _, network := range permissions.Network {
		if err := v.validateNetwork(network); err != nil {
			return fmt.Errorf("invalid network permission %q: %w", network, err)
		}
	}

	// Validate command permissions
	for _, cmd := range permissions.Commands {
		if err := v.validateCommand(cmd); err != nil {
			return fmt.Errorf("invalid command permission %q: %w", cmd, err)
		}
	}

	return nil
}

// validateFilesystemPath ensures a path is not overly broad
func (v *Validator) validateFilesystemPath(path string) error {
	// Clean path
	clean := filepath.Clean(path)

	// Disallow root filesystem access
	if clean == "/" {
		return fmt.Errorf("cannot grant access to root filesystem")
	}

	// Log suspicious permission patterns
	v.logSuspiciousPermission(path, clean)

	return nil
}

// logSuspiciousPermission logs filesystem permissions that are allowed but potentially risky
func (v *Validator) logSuspiciousPermission(original, clean string) {
	ctx := context.Background()

	// Check for home directory access
	if v.isHomeDirectoryPath(original, clean) {
		v.logger.WarnContext(ctx, "Plugin requests home directory access",
			slog.String("path", original),
			slog.String("cleaned_path", clean),
			slog.String("reason", "home_directory_access"),
			slog.String("risk", "may access sensitive files like .ssh, .env, .aws"))
		return
	}

	// Check for sensitive system directories
	if reason := v.isSensitiveSystemPath(clean); reason != "" {
		v.logger.WarnContext(ctx, "Plugin requests sensitive system directory access",
			slog.String("path", original),
			slog.String("cleaned_path", clean),
			slog.String("reason", reason),
			slog.String("risk", "system configuration or privileged files"))
		return
	}

	// Check for sensitive user directories
	if reason := v.isSensitiveUserPath(original, clean); reason != "" {
		v.logger.WarnContext(ctx, "Plugin requests sensitive user directory access",
			slog.String("path", original),
			slog.String("cleaned_path", clean),
			slog.String("reason", reason),
			slog.String("risk", "credentials or private keys"))
		return
	}

	// Check for broad/wildcard permissions
	if v.isBroadPermission(original) {
		v.logger.WarnContext(ctx, "Plugin requests broad filesystem permissions",
			slog.String("path", original),
			slog.String("cleaned_path", clean),
			slog.String("reason", "wildcard_pattern"),
			slog.String("risk", "access to multiple files/directories"))
		return
	}
}

// isHomeDirectoryPath checks if path accesses home directory
func (v *Validator) isHomeDirectoryPath(original, clean string) bool {
	return strings.Contains(original, "$HOME") || strings.HasPrefix(original, "~")
}

// isSensitiveSystemPath checks if path accesses sensitive system directories
func (v *Validator) isSensitiveSystemPath(clean string) string {
	sensitivePaths := map[string]string{
		"/etc":  "system_configuration",
		"/root": "root_home_directory",
		"/var":  "system_variable_data",
		"/sys":  "system_information",
		"/proc": "process_information",
		"/boot": "boot_configuration",
	}

	for prefix, reason := range sensitivePaths {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return reason
		}
	}

	return ""
}

// isSensitiveUserPath checks if path accesses sensitive user directories or files
func (v *Validator) isSensitiveUserPath(original, clean string) string {
	// Check for sensitive dotfile directories
	sensitiveDirs := map[string]string{
		"/.ssh":    "ssh_keys",
		"/.aws":    "aws_credentials",
		"/.gnupg":  "gpg_keys",
		"/.config": "user_configuration",
		"/.docker": "docker_credentials",
		"/.kube":   "kubernetes_credentials",
		"/.gradle": "gradle_credentials",
		"/.npm":    "npm_credentials",
	}

	for suffix, reason := range sensitiveDirs {
		if strings.Contains(clean, suffix) || strings.Contains(original, suffix) {
			return reason
		}
	}

	// Check for sensitive files (check longer patterns first to avoid false matches)
	sensitiveFiles := []string{
		".env.production", ".env.local", ".env",
		"credentials.json", "credentials.yml",
		"secrets.json", "secrets.yml",
		"id_rsa", "id_ed25519",
	}

	for _, file := range sensitiveFiles {
		if strings.Contains(clean, file) || strings.Contains(original, file) {
			return "sensitive_file_" + file
		}
	}

	return ""
}

// isBroadPermission checks if path uses wildcards or broad patterns
func (v *Validator) isBroadPermission(path string) bool {
	// Check for wildcard characters
	return strings.Contains(path, "*") || strings.Contains(path, "?")
}

// validateNetwork validates network permission format and logs all requests.
// It accepts domains, IP addresses (IPv4/IPv6), CIDR ranges, wildcard ("*"), and "localhost".
// All network permissions are logged for audit trail purposes.
// Suspicious patterns (wildcard, localhost, private IPs) generate warnings.
func (v *Validator) validateNetwork(network string) error {
	ctx := context.Background()

	// Log all network permission requests (audit trail)
	v.logger.InfoContext(ctx, "Validating network permission",
		slog.String("network", network))

	// Handle wildcard (unrestricted network access)
	if network == "*" {
		v.logWildcardNetworkAccess(ctx)
		return nil
	}

	// Handle special case: "localhost" (not a valid domain per RFC, but commonly used)
	if network == "localhost" {
		v.logger.InfoContext(ctx, "Network permission validated",
			slog.String("network", network),
			slog.String("type", "localhost"))
		v.logger.WarnContext(ctx, "Plugin requests localhost network access",
			slog.String("network", network),
			slog.String("type", "localhost"),
			slog.String("reason", "localhost_access"),
			slog.String("risk", "access to local services on loopback interface"))
		return nil
	}

	// Classify and validate permission type (domain, IP, CIDR)
	permType, err := v.classifyNetworkPermission(network)
	if err != nil {
		v.logger.ErrorContext(ctx, "Invalid network permission format",
			slog.String("network", network),
			slog.String("error", err.Error()))
		return err
	}

	// Log successful validation (audit trail)
	v.logger.InfoContext(ctx, "Network permission validated",
		slog.String("network", network),
		slog.String("type", string(permType)))

	// Check for suspicious patterns and log warnings
	v.logSuspiciousNetworkPermission(ctx, network, permType)

	return nil
}

// classifyNetworkPermission determines the type of network permission and validates format.
// Returns the permission type or an error if the format is invalid.
func (v *Validator) classifyNetworkPermission(network string) (NetworkPermissionType, error) {
	// Empty string validation
	if network == "" {
		return "", fmt.Errorf("network permission cannot be empty")
	}

	// Try parsing as IP address (handles both IPv4 and IPv6)
	if ip := net.ParseIP(network); ip != nil {
		if ip.To4() != nil {
			return PermTypeIPv4, nil
		}
		return PermTypeIPv6, nil
	}

	// Try parsing as CIDR range
	if _, _, err := net.ParseCIDR(network); err == nil {
		return PermTypeCIDR, nil
	}

	// Validate as domain name (RFC 1034/1035)
	if v.isValidDomain(network) {
		return PermTypeDomain, nil
	}

	// Invalid format
	return "", fmt.Errorf("invalid network permission: not a valid domain, IP address, or CIDR range")
}

// isValidDomain validates domain name format per RFC 1034/1035.
// Valid domains must:
//   - Be 1-253 characters total length
//   - Have at least 2 labels (domain + TLD)
//   - Each label 1-63 characters
//   - Labels contain only alphanumerics and hyphens
//   - Labels start and end with alphanumeric
//   - TLD is at least 2 letters
func (v *Validator) isValidDomain(domain string) bool {
	// Length check
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}

	// Split into labels (parts between dots)
	labels := strings.Split(domain, ".")

	// Must have at least 2 labels (e.g., "example.com")
	if len(labels) < 2 {
		return false
	}

	// Validate each label
	for _, label := range labels {
		// Label length check
		if len(label) == 0 || len(label) > 63 {
			return false
		}

		// First and last character must be alphanumeric
		if !isAlphanumeric(label[0]) || !isAlphanumeric(label[len(label)-1]) {
			return false
		}

		// All characters must be alphanumeric or hyphen
		for _, c := range label {
			if !isAlphanumeric(byte(c)) && c != '-' {
				return false
			}
		}
	}

	// TLD (last label) must be at least 2 characters and all letters
	tld := labels[len(labels)-1]
	if len(tld) < 2 {
		return false
	}
	for _, c := range tld {
		if !isLetter(byte(c)) {
			return false
		}
	}

	return true
}

// logWildcardNetworkAccess logs warning when plugin requests unrestricted network access.
// Wildcard ("*") is allowed but risky as it enables potential data exfiltration.
func (v *Validator) logWildcardNetworkAccess(ctx context.Context) {
	v.logger.WarnContext(ctx, "Plugin requests unrestricted network access",
		slog.String("network", "*"),
		slog.String("reason", "wildcard_network_access"),
		slog.String("risk", "unrestricted outbound connections, potential data exfiltration"),
		slog.String("recommendation", "specify explicit domains for better security audit trail"))
}

// logSuspiciousNetworkPermission logs warnings for suspicious network permission patterns.
// Suspicious patterns include localhost addresses, private IP ranges, and using IPs instead of domains.
func (v *Validator) logSuspiciousNetworkPermission(ctx context.Context, network string, permType NetworkPermissionType) {
	// Check for localhost addresses (127.0.0.0/8, ::1, etc.)
	if v.isLocalhostAddress(network) {
		v.logger.WarnContext(ctx, "Plugin requests localhost network access",
			slog.String("network", network),
			slog.String("type", string(permType)),
			slog.String("reason", "localhost_access"),
			slog.String("risk", "access to local services on loopback interface"))
		return
	}

	// Check for private IP ranges (RFC 1918)
	if reason := v.isPrivateIPRange(network); reason != "" {
		v.logger.WarnContext(ctx, "Plugin requests private network access",
			slog.String("network", network),
			slog.String("type", string(permType)),
			slog.String("reason", reason),
			slog.String("risk", "access to internal network resources"))
		// Don't return here - also want to log the info message about using IPs
	}

	// Info log for IP addresses (recommend using domains for better auditability)
	// This applies to all IPs, whether private or public
	if permType == PermTypeIPv4 || permType == PermTypeIPv6 {
		v.logger.InfoContext(ctx, "Plugin uses IP address instead of domain name",
			slog.String("network", network),
			slog.String("type", string(permType)),
			slog.String("reason", "ip_instead_of_domain"),
			slog.String("recommendation", "use domain names for better auditability"))
	}
}

// isLocalhostAddress checks if network permission targets localhost/loopback.
// Matches: "localhost", "127.x.x.x", "::1", "::ffff:127.x.x.x"
func (v *Validator) isLocalhostAddress(network string) bool {
	// String "localhost"
	if network == "localhost" {
		return true
	}

	// IPv6 loopback
	if network == "::1" {
		return true
	}

	// IPv4 loopback (127.0.0.0/8)
	if strings.HasPrefix(network, "127.") {
		return true
	}

	// IPv4-mapped IPv6 loopback
	if strings.HasPrefix(network, "::ffff:127.") {
		return true
	}

	return false
}

// isPrivateIPRange checks if network permission is in a private IP range per RFC 1918.
// Returns a reason string if private, empty string if public.
// Private ranges:
//   - IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
//   - IPv6: fc00::/7 (ULA), fe80::/10 (Link-Local)
func (v *Validator) isPrivateIPRange(network string) string {
	// Parse IP (handles both direct IPs and CIDR notation)
	var ip net.IP

	// Try parsing as direct IP
	if parsedIP := net.ParseIP(network); parsedIP != nil {
		ip = parsedIP
	} else {
		// Try parsing as CIDR and extract network IP
		if _, ipNet, err := net.ParseCIDR(network); err == nil {
			ip = ipNet.IP
		} else {
			// Not an IP or CIDR (probably a domain)
			return ""
		}
	}

	// Check IPv4 private ranges
	// Use To4() to get 4-byte representation, or nil if IPv6
	if ipv4 := ip.To4(); ipv4 != nil {
		// 10.0.0.0/8 (Class A)
		if ipv4[0] == 10 {
			return "private_ipv4_class_a"
		}

		// 172.16.0.0/12 (Class B)
		if ipv4[0] == 172 && ipv4[1] >= 16 && ipv4[1] <= 31 {
			return "private_ipv4_class_b"
		}

		// 192.168.0.0/16 (Class C)
		if ipv4[0] == 192 && ipv4[1] == 168 {
			return "private_ipv4_class_c"
		}
	} else {
		// Check IPv6 private ranges

		// fc00::/7 - Unique Local Addresses (ULA)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return "private_ipv6_ula"
		}

		// fe80::/10 - Link-Local
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return "private_ipv6_link_local"
		}
	}

	return ""
}

// isAlphanumeric checks if byte is alphanumeric (a-z, A-Z, 0-9)
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// isLetter checks if byte is a letter (a-z, A-Z)
func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// validateCommand ensures command is on allowlist
func (v *Validator) validateCommand(cmd string) error {
	// Command should be absolute path or well-known binary
	if !filepath.IsAbs(cmd) && !v.isWellKnownCommand(cmd) {
		return fmt.Errorf("command must be absolute path or well-known binary")
	}

	return nil
}

// isWellKnownCommand checks if command is a well-known binary
func (v *Validator) isWellKnownCommand(cmd string) bool {
	wellKnown := []string{
		"git", "gh", "npm", "node", "python", "python3",
		"make", "docker", "kubectl",
	}

	for _, known := range wellKnown {
		if cmd == known {
			return true
		}
	}

	return false
}
