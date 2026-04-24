package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	_ "github.com/mattn/go-sqlite3"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	policyJSON     bool
	policyVerbose  bool
	policyDBPath   string
	policyShowAll  bool
	policyRuleName string
)

// policyCmd represents the policy command
var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Language policy compliance dashboard and exception management",
	Long: `Language policy compliance dashboard showing:
  - Bash compliance (% files ≤20 lines)
  - Python justification rate (% files with valid imports or exceptions)
  - Validator location violations
  - Active exceptions with sunset dates
  - Expiring exceptions (within 30 days)

USAGE:
  # Show policy status dashboard
  engram policy status

  # Show exceptions
  engram policy exceptions

  # Check expiring exceptions
  engram policy expiring

  # Export as JSON
  engram policy status --json

EXCEPTION DATABASE:
  Default location: ~/src/ws/oss/repos/engram-research/swarm/projects/language-consolidation/exceptions.db
  Override with: --db=/path/to/exceptions.db or ENGRAM_EXCEPTION_DB env var`,
}

// policyStatusCmd shows overall compliance status
var policyStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show language policy compliance status",
	Long: `Display compliance dashboard with metrics:
  - Total exceptions by rule
  - Active vs expired exceptions
  - Compliance rates
  - Expiring soon alerts`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPolicyStatus()
	},
}

// policyExceptionsCmd lists all exceptions
var policyExceptionsCmd = &cobra.Command{
	Use:   "exceptions",
	Short: "List all policy exceptions",
	Long: `List all exceptions with details:
  - Exception ID
  - Rule name
  - File path
  - Status (active, approved, expired, resolved)
  - Sunset date
  - Days until sunset`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPolicyExceptions()
	},
}

// policyExpiringCmd shows expiring exceptions
var policyExpiringCmd = &cobra.Command{
	Use:   "expiring",
	Short: "Show exceptions expiring within 30 days",
	Long: `Display exceptions expiring soon:
  - Exceptions expiring in 30 days or less
  - Sorted by sunset date (soonest first)
  - Includes days until expiration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPolicyExpiring()
	},
}

func init() {
	rootCmd.AddCommand(policyCmd)
	policyCmd.AddCommand(policyStatusCmd)
	policyCmd.AddCommand(policyExceptionsCmd)
	policyCmd.AddCommand(policyExpiringCmd)

	// Global flags for policy commands
	policyCmd.PersistentFlags().BoolVar(&policyJSON, "json", false, "Output as JSON")
	policyCmd.PersistentFlags().BoolVarP(&policyVerbose, "verbose", "v", false, "Verbose output")
	policyCmd.PersistentFlags().StringVar(&policyDBPath, "db", "", "Path to exception database (default: env ENGRAM_EXCEPTION_DB or ~/src/ws/oss/repos/engram-research/swarm/projects/language-consolidation/exceptions.db)")

	// Flags specific to exceptions command
	policyExceptionsCmd.Flags().BoolVar(&policyShowAll, "all", false, "Show all exceptions including resolved")
	policyExceptionsCmd.Flags().StringVar(&policyRuleName, "rule", "", "Filter by rule name (bash-20-line-limit, python-import-justification, validator-location)")
}

// PolicyStatus represents the overall policy status
type PolicyStatus struct {
	Timestamp          time.Time         `json:"timestamp"`
	ExceptionSummary   ExceptionSummary  `json:"exception_summary"`
	ComplianceRates    ComplianceRates   `json:"compliance_rates"`
	ExpiringExceptions []ExceptionDetail `json:"expiring_exceptions"`
	RuleBreakdown      []RuleBreakdown   `json:"rule_breakdown"`
}

// ExceptionSummary contains aggregate exception counts
type ExceptionSummary struct {
	TotalActive   int `json:"total_active"`
	TotalExpired  int `json:"total_expired"`
	TotalResolved int `json:"total_resolved"`
	TotalPending  int `json:"total_pending"`
	Total         int `json:"total"`
}

// ComplianceRates contains compliance percentages
type ComplianceRates struct {
	BashCompliance      string `json:"bash_compliance"`
	PythonCompliance    string `json:"python_compliance"`
	ValidatorCompliance string `json:"validator_compliance"`
}

// RuleBreakdown contains per-rule statistics
type RuleBreakdown struct {
	RuleName       string `json:"rule_name"`
	Active         int    `json:"active"`
	Expired        int    `json:"expired"`
	Resolved       int    `json:"resolved"`
	Total          int    `json:"total"`
	EarliestSunset string `json:"earliest_sunset"`
}

// ExceptionDetail contains detailed exception information
type ExceptionDetail struct {
	ID              int    `json:"id"`
	RuleName        string `json:"rule_name"`
	FilePath        string `json:"file_path"`
	Status          string `json:"status"`
	Reason          string `json:"reason"`
	Approver        string `json:"approver"`
	ApprovedDate    string `json:"approved_date"`
	SunsetDate      string `json:"sunset_date"`
	DaysUntilSunset int    `json:"days_until_sunset"`
	GitHubIssueURL  string `json:"github_issue_url,omitempty"`
}

func getExceptionDBPath() string {
	if policyDBPath != "" {
		return policyDBPath
	}

	// Check environment variable
	if dbPath := os.Getenv("ENGRAM_EXCEPTION_DB"); dbPath != "" {
		return dbPath
	}

	// Default path
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "src/ws/oss/repos/engram-research/swarm/projects/language-consolidation/exceptions.db")
}

func openExceptionDB() (*sql.DB, error) {
	dbPath := getExceptionDBPath()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("exception database not found: %s\n\nPlease ensure the language-consolidation project is initialized.\nSet ENGRAM_EXCEPTION_DB env var or use --db flag to specify location.", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open exception database: %w", err)
	}

	return db, nil
}

func runPolicyStatus() error {
	db, err := openExceptionDB()
	if err != nil {
		return err
	}
	defer db.Close()

	status := PolicyStatus{
		Timestamp: time.Now(),
	}

	// Get exception summary
	if err := getExceptionSummary(db, &status.ExceptionSummary); err != nil {
		return fmt.Errorf("failed to get exception summary: %w", err)
	}

	// Get compliance rates
	if err := getComplianceRates(db, &status.ComplianceRates); err != nil {
		return fmt.Errorf("failed to get compliance rates: %w", err)
	}

	// Get expiring exceptions (within 30 days)
	if err := getExpiringExceptions(db, &status.ExpiringExceptions, 30); err != nil {
		return fmt.Errorf("failed to get expiring exceptions: %w", err)
	}

	// Get rule breakdown
	if err := getRuleBreakdown(db, &status.RuleBreakdown); err != nil {
		return fmt.Errorf("failed to get rule breakdown: %w", err)
	}

	// Output
	if policyJSON {
		return outputPolicyJSON(status)
	}

	return outputStatusTable(status)
}

func getExceptionSummary(db *sql.DB, summary *ExceptionSummary) error {
	query := `
		SELECT
			COUNT(CASE WHEN status IN ('active', 'approved') THEN 1 END) as active,
			COUNT(CASE WHEN status = 'expired' THEN 1 END) as expired,
			COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved,
			COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending,
			COUNT(*) as total
		FROM exceptions
	`

	row := db.QueryRow(query)
	err := row.Scan(&summary.TotalActive, &summary.TotalExpired, &summary.TotalResolved, &summary.TotalPending, &summary.Total)
	return err
}

func getComplianceRates(db *sql.DB, rates *ComplianceRates) error {
	// Get total files with exceptions per rule
	query := `
		SELECT
			rule_name,
			COUNT(*) as count
		FROM exceptions
		WHERE status IN ('active', 'approved')
		GROUP BY rule_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	ruleCounts := make(map[string]int)
	for rows.Next() {
		var ruleName string
		var count int
		if err := rows.Scan(&ruleName, &count); err != nil {
			return err
		}
		ruleCounts[ruleName] = count
	}

	// Calculate compliance rates (simplified - actual implementation would scan repos)
	// For now, we show exception counts
	bashExceptions := ruleCounts["bash-20-line-limit"]
	pythonExceptions := ruleCounts["python-import-justification"]
	validatorExceptions := ruleCounts["validator-location"]

	// Simplified compliance display
	rates.BashCompliance = fmt.Sprintf("%d active exceptions", bashExceptions)
	rates.PythonCompliance = fmt.Sprintf("%d active exceptions", pythonExceptions)
	rates.ValidatorCompliance = fmt.Sprintf("%d active exceptions", validatorExceptions)

	return nil
}

func getExpiringExceptions(db *sql.DB, exceptions *[]ExceptionDetail, days int) error {
	query := `
		SELECT
			id,
			rule_name,
			file_path,
			status,
			reason,
			approver,
			approved_date,
			sunset_date,
			CAST(JULIANDAY(sunset_date) - JULIANDAY('now') AS INTEGER) as days_until_sunset,
			COALESCE(github_issue_url, '') as github_issue_url
		FROM exceptions
		WHERE status IN ('active', 'approved')
		  AND sunset_date >= DATE('now')
		  AND sunset_date <= DATE('now', '+' || ? || ' days')
		ORDER BY sunset_date ASC
	`

	rows, err := db.Query(query, days)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var exc ExceptionDetail
		err := rows.Scan(
			&exc.ID,
			&exc.RuleName,
			&exc.FilePath,
			&exc.Status,
			&exc.Reason,
			&exc.Approver,
			&exc.ApprovedDate,
			&exc.SunsetDate,
			&exc.DaysUntilSunset,
			&exc.GitHubIssueURL,
		)
		if err != nil {
			return err
		}
		*exceptions = append(*exceptions, exc)
	}

	return rows.Err()
}

func getRuleBreakdown(db *sql.DB, breakdown *[]RuleBreakdown) error {
	query := `
		SELECT
			rule_name,
			COUNT(CASE WHEN status IN ('active', 'approved') THEN 1 END) as active,
			COUNT(CASE WHEN status = 'expired' THEN 1 END) as expired,
			COUNT(CASE WHEN status = 'resolved' THEN 1 END) as resolved,
			COUNT(*) as total,
			MIN(CASE WHEN status IN ('active', 'approved') THEN sunset_date END) as earliest_sunset
		FROM exceptions
		GROUP BY rule_name
		ORDER BY rule_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var rb RuleBreakdown
		var earliestSunset sql.NullString
		err := rows.Scan(&rb.RuleName, &rb.Active, &rb.Expired, &rb.Resolved, &rb.Total, &earliestSunset)
		if err != nil {
			return err
		}
		if earliestSunset.Valid {
			rb.EarliestSunset = earliestSunset.String
		} else {
			rb.EarliestSunset = "N/A"
		}
		*breakdown = append(*breakdown, rb)
	}

	return rows.Err()
}

func outputPolicyJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func outputStatusTable(status PolicyStatus) error {
	// Print header
	bold := color.New(color.Bold)
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)

	bold.Println("\n╔════════════════════════════════════════════════════════════════════╗")
	bold.Println("║          Language Policy Compliance Dashboard                     ║")
	bold.Println("╚════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("Generated: %s\n\n", status.Timestamp.Format("2006-01-02 15:04:05"))

	// Exception Summary
	bold.Println("═══ EXCEPTION SUMMARY ═══")
	fmt.Printf("Total Exceptions:   %d\n", status.ExceptionSummary.Total)
	green.Printf("  ✓ Active:         %d\n", status.ExceptionSummary.TotalActive)
	yellow.Printf("  ⚠ Pending:        %d\n", status.ExceptionSummary.TotalPending)
	red.Printf("  ✗ Expired:        %d\n", status.ExceptionSummary.TotalExpired)
	fmt.Printf("  ✓ Resolved:       %d\n\n", status.ExceptionSummary.TotalResolved)

	// Compliance Rates
	bold.Println("═══ COMPLIANCE STATUS ═══")
	fmt.Printf("bash-20-line-limit:           %s\n", status.ComplianceRates.BashCompliance)
	fmt.Printf("python-import-justification:  %s\n", status.ComplianceRates.PythonCompliance)
	fmt.Printf("validator-location:           %s\n\n", status.ComplianceRates.ValidatorCompliance)

	// Rule Breakdown
	bold.Println("═══ RULE BREAKDOWN ═══")
	table := tablewriter.NewWriter(os.Stdout)
	table.Header("Rule", "Active", "Expired", "Resolved", "Total", "Earliest Sunset")

	for _, rb := range status.RuleBreakdown {
		// Shorten rule name for display
		shortName := shortenRuleName(rb.RuleName)
		table.Append(
			shortName,
			fmt.Sprintf("%d", rb.Active),
			fmt.Sprintf("%d", rb.Expired),
			fmt.Sprintf("%d", rb.Resolved),
			fmt.Sprintf("%d", rb.Total),
			rb.EarliestSunset,
		)
	}
	table.Render()
	fmt.Println()

	// Expiring Exceptions
	if len(status.ExpiringExceptions) > 0 {
		yellow.Printf("\n⚠ EXPIRING SOON (within 30 days): %d exceptions\n\n", len(status.ExpiringExceptions))

		expTable := tablewriter.NewWriter(os.Stdout)
		expTable.Header("ID", "Rule", "File", "Sunset Date", "Days Left")

		for _, exc := range status.ExpiringExceptions {
			shortRule := shortenRuleName(exc.RuleName)
			shortFile := shortenPath(exc.FilePath, 40)

			// Color code by urgency
			var daysStr string
			if exc.DaysUntilSunset <= 7 {
				daysStr = red.Sprintf("%d", exc.DaysUntilSunset)
			} else if exc.DaysUntilSunset <= 14 {
				daysStr = yellow.Sprintf("%d", exc.DaysUntilSunset)
			} else {
				daysStr = fmt.Sprintf("%d", exc.DaysUntilSunset)
			}

			expTable.Append(
				fmt.Sprintf("#%d", exc.ID),
				shortRule,
				shortFile,
				exc.SunsetDate,
				daysStr,
			)
		}
		expTable.Render()
		fmt.Println()
	} else {
		green.Println("✓ No exceptions expiring in the next 30 days")
	}

	// Footer
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("Run 'engram policy exceptions' to see all exceptions")
	fmt.Println("Run 'engram policy expiring' to see only expiring exceptions")
	fmt.Println("Run 'engram policy status --json' for machine-readable output")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()

	return nil
}

func runPolicyExceptions() error {
	db, err := openExceptionDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Build query
	query := `
		SELECT
			id,
			rule_name,
			file_path,
			status,
			reason,
			approver,
			approved_date,
			sunset_date,
			CAST(JULIANDAY(sunset_date) - JULIANDAY('now') AS INTEGER) as days_until_sunset,
			COALESCE(github_issue_url, '') as github_issue_url
		FROM exceptions
		WHERE 1=1
	`

	var args []interface{}

	// Filter by status (exclude resolved unless --all)
	if !policyShowAll {
		query += " AND status != 'resolved'"
	}

	// Filter by rule name
	if policyRuleName != "" {
		query += " AND rule_name = ?"
		args = append(args, policyRuleName)
	}

	query += " ORDER BY sunset_date ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var exceptions []ExceptionDetail
	for rows.Next() {
		var exc ExceptionDetail
		err := rows.Scan(
			&exc.ID,
			&exc.RuleName,
			&exc.FilePath,
			&exc.Status,
			&exc.Reason,
			&exc.Approver,
			&exc.ApprovedDate,
			&exc.SunsetDate,
			&exc.DaysUntilSunset,
			&exc.GitHubIssueURL,
		)
		if err != nil {
			return err
		}
		exceptions = append(exceptions, exc)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	// Output
	if policyJSON {
		return outputPolicyJSON(exceptions)
	}

	return outputExceptionsTable(exceptions)
}

func outputExceptionsTable(exceptions []ExceptionDetail) error {
	bold := color.New(color.Bold)

	bold.Println("\n╔════════════════════════════════════════════════════════════════════╗")
	bold.Println("║                    Policy Exceptions                               ║")
	bold.Println("╚════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("Total: %d exceptions\n\n", len(exceptions))

	if len(exceptions) == 0 {
		fmt.Println("No exceptions found.")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("ID", "Rule", "File", "Status", "Sunset", "Days Left")

	for _, exc := range exceptions {
		shortRule := shortenRuleName(exc.RuleName)
		shortFile := shortenPath(exc.FilePath, 40)

		// Color code status
		var statusStr string
		switch exc.Status {
		case "active", "approved":
			statusStr = color.GreenString(exc.Status)
		case "expired":
			statusStr = color.RedString(exc.Status)
		case "pending":
			statusStr = color.YellowString(exc.Status)
		default:
			statusStr = exc.Status
		}

		// Format days left
		var daysStr string
		if exc.Status == "expired" || exc.Status == "resolved" {
			daysStr = "-"
		} else if exc.DaysUntilSunset < 0 {
			daysStr = color.RedString("EXPIRED")
		} else if exc.DaysUntilSunset <= 7 {
			daysStr = color.RedString("%d", exc.DaysUntilSunset)
		} else if exc.DaysUntilSunset <= 30 {
			daysStr = color.YellowString("%d", exc.DaysUntilSunset)
		} else {
			daysStr = fmt.Sprintf("%d", exc.DaysUntilSunset)
		}

		table.Append(
			fmt.Sprintf("#%d", exc.ID),
			shortRule,
			shortFile,
			statusStr,
			exc.SunsetDate,
			daysStr,
		)

		// Verbose mode: print details
		if policyVerbose {
			fmt.Printf("\n[#%d] %s: %s\n", exc.ID, exc.RuleName, exc.FilePath)
			fmt.Printf("  Status: %s\n", exc.Status)
			fmt.Printf("  Approver: %s\n", exc.Approver)
			fmt.Printf("  Approved: %s\n", exc.ApprovedDate)
			fmt.Printf("  Sunset: %s\n", exc.SunsetDate)
			if exc.GitHubIssueURL != "" {
				fmt.Printf("  Issue: %s\n", exc.GitHubIssueURL)
			}
			fmt.Printf("  Reason: %s\n", truncate(exc.Reason, 80))
		}
	}

	table.Render()
	fmt.Println()

	if !policyVerbose {
		fmt.Println("Run with --verbose for detailed information")
	}

	return nil
}

func runPolicyExpiring() error {
	db, err := openExceptionDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var exceptions []ExceptionDetail
	if err := getExpiringExceptions(db, &exceptions, 30); err != nil {
		return fmt.Errorf("failed to get expiring exceptions: %w", err)
	}

	// Output
	if policyJSON {
		return outputPolicyJSON(exceptions)
	}

	bold := color.New(color.Bold)
	yellow := color.New(color.FgYellow)

	bold.Println("\n╔════════════════════════════════════════════════════════════════════╗")
	bold.Println("║              Exceptions Expiring Within 30 Days                    ║")
	bold.Println("╚════════════════════════════════════════════════════════════════════╝")

	if len(exceptions) == 0 {
		fmt.Println()
		fmt.Println("✓ No exceptions expiring in the next 30 days")
		fmt.Println()
		return nil
	}

	yellow.Printf("\n⚠ %d exceptions expiring soon\n\n", len(exceptions))

	return outputExceptionsTable(exceptions)
}

// Helper functions

func shortenRuleName(ruleName string) string {
	switch ruleName {
	case "bash-20-line-limit":
		return "bash-20-line"
	case "python-import-justification":
		return "python-import"
	case "validator-location":
		return "validator-loc"
	default:
		return ruleName
	}
}

func shortenPath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Try to keep filename and some parent context
	parts := strings.Split(path, "/")
	filename := parts[len(parts)-1]

	if len(filename) >= maxLen-3 {
		return "..." + filename[len(filename)-(maxLen-3):]
	}

	// Build path from end until we hit max
	result := filename
	for i := len(parts) - 2; i >= 0; i-- {
		candidate := parts[i] + "/" + result
		if len(candidate) > maxLen-3 {
			return "..." + result
		}
		result = candidate
	}

	return result
}
