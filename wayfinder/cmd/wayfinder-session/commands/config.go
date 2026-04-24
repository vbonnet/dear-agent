package commands

var projectDirectory string

// SetProjectDirectory stores the project directory for use by all commands
func SetProjectDirectory(dir string) {
	projectDirectory = dir
}

// GetProjectDirectory returns the project directory (defaults to "." if not set)
func GetProjectDirectory() string {
	if projectDirectory == "" {
		return "."
	}
	return projectDirectory
}
