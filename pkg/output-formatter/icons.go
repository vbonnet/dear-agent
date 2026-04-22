package outputformatter

// IconMapper converts status levels to visual icons
type IconMapper struct {
	NoColor bool // If true, returns plain text instead of emojis
}

// NewIconMapper creates a new icon mapper
func NewIconMapper(noColor bool) *IconMapper {
	return &IconMapper{NoColor: noColor}
}

// GetIcon returns the icon for a given status level
func (m *IconMapper) GetIcon(status StatusLevel) string {
	if m.NoColor {
		return m.getPlainIcon(status)
	}
	return m.getEmojiIcon(status)
}

// getEmojiIcon returns emoji icons for status levels
func (m *IconMapper) getEmojiIcon(status StatusLevel) string {
	switch status {
	case StatusOK, StatusSuccess:
		return "✅"
	case StatusInfo:
		return "ℹ️ "
	case StatusWarning:
		return "⚠️ "
	case StatusError, StatusFailed:
		return "❌"
	case StatusUnknown:
		return "❓"
	default:
		return "  " // Two spaces for alignment
	}
}

// getPlainIcon returns plain text icons for accessibility
func (m *IconMapper) getPlainIcon(status StatusLevel) string {
	switch status {
	case StatusOK, StatusSuccess:
		return "[OK]"
	case StatusInfo:
		return "[INFO]"
	case StatusWarning:
		return "[WARN]"
	case StatusError, StatusFailed:
		return "[ERROR]"
	case StatusUnknown:
		return "[?]"
	default:
		return "     " // Five spaces for alignment with [TEXT]
	}
}

// FormatWithIcon returns a formatted string with icon prefix
func (m *IconMapper) FormatWithIcon(status StatusLevel, message string) string {
	icon := m.GetIcon(status)
	return icon + " " + message
}
