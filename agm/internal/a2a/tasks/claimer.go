// Package tasks provides tasks-related functionality.
package tasks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/config"
)

// Claimer handles task claiming and ownership tracking
type Claimer struct {
	channelsDir string
	activeDir   string
}

// NewClaimer creates a new task claimer
func NewClaimer(channelsDir string) *Claimer {
	if channelsDir == "" {
		channelsPath := config.GetChannelsDir()
		if filepath.Base(channelsPath) == "active" {
			channelsDir = filepath.Dir(channelsPath)
		} else {
			channelsDir = channelsPath
		}
	}

	activeDir := filepath.Join(channelsDir, "active")

	return &Claimer{
		channelsDir: channelsDir,
		activeDir:   activeDir,
	}
}

// ClaimTask atomically claims unclaimed task
func (c *Claimer) ClaimTask(channelID, agentID, reason string) (bool, error) {
	channelFile := filepath.Join(c.activeDir, channelID+".md")
	lockFile := filepath.Join(c.activeDir, "."+channelID+".lock")

	if _, err := os.Stat(channelFile); os.IsNotExist(err) {
		return false, fmt.Errorf("channel not found: %s", channelID)
	}

	lockFd, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, fmt.Errorf("failed to open lock file: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFd.Fd()), syscall.LOCK_UN)
		lockFd.Close()
		os.Remove(lockFile)
	}()

	if err := acquireLockWithTimeout(lockFd, 5*time.Second); err != nil {
		return false, fmt.Errorf("failed to acquire lock on %s: %w", channelID, err)
	}

	content, err := os.ReadFile(channelFile)
	if err != nil {
		return false, fmt.Errorf("failed to read channel: %w", err)
	}

	contentStr := string(content)

	if isClaimed(contentStr) {
		owner := getOwner(contentStr)
		return false, fmt.Errorf("task already claimed by: %s", owner)
	}

	timestamp := time.Now().Format("2006-01-02 15:04")
	updated := injectOwner(contentStr, agentID, timestamp, reason)
	updated = updateLatestStatus(updated, "in-progress")

	if err := os.WriteFile(channelFile, []byte(updated), 0o600); err != nil {
		return false, fmt.Errorf("failed to update channel: %w", err)
	}

	return true, nil
}

// UnclaimTask releases claimed task
func (c *Claimer) UnclaimTask(channelID, agentID, reason string) error {
	channelFile := filepath.Join(c.activeDir, channelID+".md")
	lockFile := filepath.Join(c.activeDir, "."+channelID+".lock")

	if _, err := os.Stat(channelFile); os.IsNotExist(err) {
		return fmt.Errorf("channel not found: %s", channelID)
	}

	lockFd, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFd.Fd()), syscall.LOCK_UN)
		lockFd.Close()
		os.Remove(lockFile)
	}()

	if err := acquireLockWithTimeout(lockFd, 5*time.Second); err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	content, err := os.ReadFile(channelFile)
	if err != nil {
		return fmt.Errorf("failed to read channel: %w", err)
	}

	contentStr := string(content)

	currentOwner := getOwner(contentStr)
	if currentOwner != agentID {
		return fmt.Errorf("only owner (%s) can unclaim. You are: %s", currentOwner, agentID)
	}

	updated := removeOwner(contentStr, reason)
	updated = updateLatestStatus(updated, "awaiting-response")

	if err := os.WriteFile(channelFile, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("failed to update channel: %w", err)
	}

	return nil
}

// TaskInfo represents claimable task metadata
type TaskInfo struct {
	ChannelID   string
	Topic       string
	PostedBy    string
	Timestamp   string
	Description string
}

// ListClaimableTasks lists all unclaimed tasks
func (c *Claimer) ListClaimableTasks() ([]*TaskInfo, error) {
	var claimable []*TaskInfo

	if _, err := os.Stat(c.activeDir); os.IsNotExist(err) {
		return claimable, nil
	}

	matches, err := filepath.Glob(filepath.Join(c.activeDir, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob channels: %w", err)
	}

	for _, channelFile := range matches {
		base := filepath.Base(channelFile)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "INSTRUCTIONS") {
			continue
		}

		content, err := os.ReadFile(channelFile)
		if err != nil {
			continue
		}

		contentStr := string(content)

		if isClaimed(contentStr) {
			continue
		}

		header := parseLatestMessageHeader(contentStr)
		if header == nil {
			continue
		}

		if header["status"] == "awaiting-response" {
			description := extractProposal(contentStr)
			if len(description) > 150 {
				description = description[:150]
			}

			claimable = append(claimable, &TaskInfo{
				ChannelID:   strings.TrimSuffix(base, ".md"),
				Topic:       extractTopic(contentStr),
				PostedBy:    header["agent_id"],
				Timestamp:   header["timestamp"],
				Description: description,
			})
		}
	}

	return claimable, nil
}

func acquireLockWithTimeout(fd *os.File, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("lock acquisition timeout after %v", timeout)
}

func isClaimed(content string) bool {
	return strings.Contains(content, "**Owner**:")
}

func getOwner(content string) string {
	re := regexp.MustCompile(`\*\*Owner\*\*:\s*(.+)`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func injectOwner(content, agentID, timestamp, reason string) string {
	lines := strings.Split(content, "\n")
	var separatorIndices []int
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIndices = append(separatorIndices, i)
		}
	}

	if len(separatorIndices) < 2 {
		return content
	}

	headerEnd := separatorIndices[1]

	ownerBlock := []string{
		fmt.Sprintf("**Owner**: %s", agentID),
		fmt.Sprintf("**Claimed**: %s", timestamp),
	}
	if reason != "" {
		ownerBlock = append(ownerBlock, fmt.Sprintf("**Claim Reason**: %s", reason))
	}

	result := make([]string, 0, len(lines)+len(ownerBlock))
	result = append(result, lines[:headerEnd]...)
	result = append(result, ownerBlock...)
	result = append(result, lines[headerEnd:]...)

	return strings.Join(result, "\n")
}

func removeOwner(content, reason string) string {
	lines := strings.Split(content, "\n")

	var filtered []string
	for _, line := range lines {
		if !strings.Contains(line, "**Owner**:") &&
			!strings.Contains(line, "**Claimed**:") &&
			!strings.Contains(line, "**Claim Reason**:") {
			filtered = append(filtered, line)
		}
	}

	if reason != "" {
		var separatorIndices []int
		for i, line := range filtered {
			if strings.TrimSpace(line) == "---" {
				separatorIndices = append(separatorIndices, i)
			}
		}

		if len(separatorIndices) >= 2 {
			headerEnd := separatorIndices[1]
			timestamp := time.Now().Format("2006-01-02 15:04")
			unclaimLine := fmt.Sprintf("**Unclaimed**: %s - %s", timestamp, reason)

			result := make([]string, 0, len(filtered)+1)
			result = append(result, filtered[:headerEnd]...)
			result = append(result, unclaimLine)
			result = append(result, filtered[headerEnd:]...)
			filtered = result
		}
	}

	return strings.Join(filtered, "\n")
}

func updateLatestStatus(content, newStatus string) string {
	lines := strings.Split(content, "\n")

	var separatorIndices []int
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIndices = append(separatorIndices, i)
		}
	}

	if len(separatorIndices) < 3 {
		return content
	}

	headerStart := separatorIndices[len(separatorIndices)-3]
	headerEnd := separatorIndices[len(separatorIndices)-2]

	for i := headerStart + 1; i < headerEnd; i++ {
		if strings.Contains(lines[i], "**Status**:") {
			lines[i] = fmt.Sprintf("**Status**: %s", newStatus)
			break
		}
	}

	return strings.Join(lines, "\n")
}

func parseLatestMessageHeader(content string) map[string]string {
	lines := strings.Split(strings.TrimSpace(content), "\n")

	var separatorIndices []int
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIndices = append(separatorIndices, i)
		}
	}

	if len(separatorIndices) < 3 {
		return nil
	}

	headerStart := separatorIndices[len(separatorIndices)-3]
	headerEnd := separatorIndices[len(separatorIndices)-2]

	header := make(map[string]string)

	for i := headerStart + 1; i < headerEnd && i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		switch {
		case strings.HasPrefix(line, "**Agent ID**:"):
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				header["agent_id"] = strings.TrimSpace(parts[1])
			}
		case strings.HasPrefix(line, "**Timestamp**:"):
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				header["timestamp"] = strings.TrimSpace(parts[1])
			}
		case strings.HasPrefix(line, "**Status**:"):
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				header["status"] = strings.TrimSpace(parts[1])
			}
		}
	}

	return header
}

func extractTopic(content string) string {
	re := regexp.MustCompile(`\*\*Topic\*\*:\s*(.+)`)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func extractProposal(content string) string {
	lines := strings.Split(content, "\n")

	proposalStart := -1
	for i, line := range lines {
		if strings.Contains(line, "### Proposal") || strings.Contains(line, "### Response") {
			proposalStart = i + 1
			break
		}
	}

	if proposalStart < 0 || proposalStart >= len(lines) {
		return ""
	}

	var proposalLines []string
	for i := proposalStart; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "###") || strings.TrimSpace(lines[i]) == "---" {
			break
		}
		if strings.TrimSpace(lines[i]) != "" {
			proposalLines = append(proposalLines, strings.TrimSpace(lines[i]))
		}
	}

	return strings.Join(proposalLines, " ")
}
