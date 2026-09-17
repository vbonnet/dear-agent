package steps

import "testing"

func TestRequireUnskippedGoTestPasses(t *testing.T) {
	t.Run("accepts exact parent and subtest passes", func(t *testing.T) {
		output := "--- PASS: TestSecurity (0.01s)\n    --- PASS: TestSecurity/symlink (0.00s)\n"
		if err := requireUnskippedGoTestPasses("security", output, "TestSecurity", "TestSecurity/symlink"); err != nil {
			t.Fatalf("requireUnskippedGoTestPasses() error = %v", err)
		}
	})

	t.Run("rejects a skipped subtest despite a passing parent", func(t *testing.T) {
		output := "--- PASS: TestSecurity (0.01s)\n    --- SKIP: TestSecurity/fifo (0.00s)\n"
		if err := requireUnskippedGoTestPasses("security", output, "TestSecurity"); err == nil {
			t.Fatal("requireUnskippedGoTestPasses() error = nil, want skipped-proof rejection")
		}
	})

	t.Run("rejects a longer test-name prefix", func(t *testing.T) {
		output := "--- PASS: TestSecurityExtra (0.01s)\n"
		if err := requireUnskippedGoTestPasses("security", output, "TestSecurity"); err == nil {
			t.Fatal("requireUnskippedGoTestPasses() error = nil, want exact-name rejection")
		}
	})
}
