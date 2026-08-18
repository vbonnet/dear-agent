package ops

import (
	"fmt"
)

func sessionNameExistsMessage(name string) string {
	return fmt.Sprintf("session '%s' already exists. Use a different name or archive the existing session with: agm session archive %s", name, name)
}
