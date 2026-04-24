package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

type DiaryEvent struct {
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"`
	Session   string    `json:"session"`
	Summary   string    `json:"summary"`
}

var diaryCmd = &cobra.Command{
	Use:   "diary [session]",
	Short: "View dear-diary session event log",
	RunE: func(cmd *cobra.Command, args []string) error {
		homeDir, _ := os.UserHomeDir()
		diaryPath := filepath.Join(homeDir, ".agm", "logs", "diary.jsonl")

		data, err := os.ReadFile(diaryPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No diary entries yet.")
				return nil
			}
			return err
		}

		fmt.Println("=== dear-diary ===")
		for _, line := range splitLines(data) {
			var event DiaryEvent
			if json.Unmarshal(line, &event) == nil {
				if len(args) == 0 || event.Session == args[0] {
					fmt.Printf("[%s] %s: %s - %s\n",
						event.Timestamp.Format("15:04"),
						event.Type, event.Session, event.Summary)
				}
			}
		}
		return nil
	},
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

func init() {
	rootCmd.AddCommand(diaryCmd)
}
