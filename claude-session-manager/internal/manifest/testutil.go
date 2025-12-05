package manifest

import "time"

func testTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2025-12-05T10:00:00Z")
	return t
}
