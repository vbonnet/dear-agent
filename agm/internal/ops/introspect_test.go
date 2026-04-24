package ops

import "testing"

func TestListOps_ReturnsOperations(t *testing.T) {
	result := ListOps()

	if result == nil {
		t.Fatal("ListOps() returned nil")
	}

	if result.Operation != "list_ops" {
		t.Errorf("Operation = %q, want %q", result.Operation, "list_ops")
	}

	if len(result.Operations) == 0 {
		t.Fatal("Operations slice is empty")
	}

	if result.Total != len(result.Operations) {
		t.Errorf("Total = %d, want %d (len of Operations)", result.Total, len(result.Operations))
	}

	for i, op := range result.Operations {
		if op.Name == "" {
			t.Errorf("Operations[%d].Name is empty", i)
		}
		if op.Description == "" {
			t.Errorf("Operations[%d].Description is empty (name=%s)", i, op.Name)
		}
		if op.Category == "" {
			t.Errorf("Operations[%d].Category is empty (name=%s)", i, op.Name)
		}
		if op.Surface == "" {
			t.Errorf("Operations[%d].Surface is empty (name=%s)", i, op.Name)
		}

		// Validate category values
		switch op.Category {
		case "read", "mutation", "meta":
			// valid
		default:
			t.Errorf("Operations[%d].Category = %q, want one of read/mutation/meta (name=%s)", i, op.Category, op.Name)
		}
	}
}
