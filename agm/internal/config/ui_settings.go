package config

// UISettings is the UI-owned projection of AGM's shared configuration file.
// Keeping it in this package lets one strict schema validate the complete
// document before runtime and presentation code consume their projections.
type UISettings struct {
	Defaults DefaultsConfig `yaml:"defaults"`
	UI       UIConfig       `yaml:"ui"`
}

// DefaultsConfig holds default behavior toggles for the AGM UI.
type DefaultsConfig struct {
	Interactive          bool `yaml:"interactive"`
	AutoAssociateUUID    bool `yaml:"auto_associate_uuid"`
	ConfirmDestructive   bool `yaml:"confirm_destructive"`
	CleanupThresholdDays int  `yaml:"cleanup_threshold_days"`
	ArchiveThresholdDays int  `yaml:"archive_threshold_days"`
}

// UIConfig holds presentation and accessibility settings.
type UIConfig struct {
	Theme            string `yaml:"theme"`
	PickerHeight     int    `yaml:"picker_height"`
	ShowProjectPaths bool   `yaml:"show_project_paths"`
	ShowTags         bool   `yaml:"show_tags"`
	FuzzySearch      bool   `yaml:"fuzzy_search"`
	NoColor          bool   `yaml:"no_color"`
	ScreenReader     bool   `yaml:"screen_reader"`
}

// DefaultUISettings returns the shared UI defaults projected from Config.
func DefaultUISettings() UISettings {
	return UISettings{
		Defaults: DefaultsConfig{
			Interactive:          true,
			AutoAssociateUUID:    true,
			ConfirmDestructive:   true,
			CleanupThresholdDays: 30,
			ArchiveThresholdDays: 90,
		},
		UI: UIConfig{
			Theme:            "agm",
			PickerHeight:     15,
			ShowProjectPaths: true,
			ShowTags:         true,
			FuzzySearch:      true,
		},
	}
}

// normalizeUISettings preserves the legacy zero-as-default behavior for the
// four scalar values that the former UI-only loader normalized after decode.
func normalizeUISettings(settings *UISettings) {
	defaults := DefaultUISettings()
	if settings.UI.PickerHeight == 0 {
		settings.UI.PickerHeight = defaults.UI.PickerHeight
	}
	if settings.Defaults.CleanupThresholdDays == 0 {
		settings.Defaults.CleanupThresholdDays = defaults.Defaults.CleanupThresholdDays
	}
	if settings.Defaults.ArchiveThresholdDays == 0 {
		settings.Defaults.ArchiveThresholdDays = defaults.Defaults.ArchiveThresholdDays
	}
	if settings.UI.Theme == "" {
		settings.UI.Theme = defaults.UI.Theme
	}
}
