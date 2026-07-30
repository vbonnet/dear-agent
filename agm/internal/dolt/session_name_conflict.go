package dolt

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

const nonArchivedSessionNameIndex = "uq_agm_sessions_workspace_non_archived_name"

// SessionNameConflictError reports an atomic durable collision on a
// non-archived session name.
type SessionNameConflictError struct {
	Name string
}

func (e *SessionNameConflictError) Error() string {
	return fmt.Sprintf("non-archived session name already exists: %s", e.Name)
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, nonArchivedSessionNameIndex) ||
		strings.Contains(message, "unique constraint failed")
}
