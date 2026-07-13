package main

import (
	"reflect"
	"testing"
)

func TestJobsForFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{name: "none", files: nil, want: nil},
		{
			name:  "core adds test and lint in lexical order",
			files: []string{"core/runtime.go"},
			want:  []string{"lint", "test"},
		},
		{
			name:  "AGM and Go changes deduplicate lint",
			files: []string{"agm/main.go", "engram/config.go"},
			want:  []string{"lint", "unit-tests"},
		},
		{
			name:  "unrelated files do not select jobs",
			files: []string{"README.md"},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobsForFiles(tt.files); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("jobsForFiles(%v) = %v, want %v", tt.files, got, tt.want)
			}
		})
	}
}
