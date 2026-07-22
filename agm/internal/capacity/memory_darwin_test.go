//go:build darwin

package capacity

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestReadDarwinMemoryInfo(t *testing.T) {
	const total = uint64(32 * 1024 * 1024 * 1024)
	gotTotal, gotAvailable, err := readDarwinMemoryInfo(
		func() (uint64, error) { return total, nil },
		func() (float64, error) { return 62.5, nil },
	)
	if err != nil {
		t.Fatalf("readDarwinMemoryInfo() error: %v", err)
	}
	if gotTotal != total {
		t.Errorf("total = %d, want %d", gotTotal, total)
	}
	wantAvailable := uint64(20 * 1024 * 1024 * 1024)
	if gotAvailable != wantAvailable {
		t.Errorf("available = %d, want %d", gotAvailable, wantAvailable)
	}
}

func TestReadDarwinMemoryInfoErrors(t *testing.T) {
	tests := []struct {
		name        string
		totalReader func() (uint64, error)
		pctReader   func() (float64, error)
		want        string
	}{
		{
			name:        "sysctl failure",
			totalReader: func() (uint64, error) { return 0, errors.New("denied") },
			pctReader:   func() (float64, error) { return 50, nil },
			want:        "reading hw.memsize",
		},
		{
			name:        "zero total",
			totalReader: func() (uint64, error) { return 0, nil },
			pctReader:   func() (float64, error) { return 50, nil },
			want:        "returned zero bytes",
		},
		{
			name:        "pressure failure",
			totalReader: func() (uint64, error) { return 32 << 30, nil },
			pctReader:   func() (float64, error) { return 0, errors.New("timed out") },
			want:        "reading memory pressure",
		},
		{
			name:        "invalid pressure",
			totalReader: func() (uint64, error) { return 32 << 30, nil },
			pctReader:   func() (float64, error) { return math.NaN(), nil },
			want:        "outside [0, 100]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := readDarwinMemoryInfo(tt.totalReader, tt.pctReader)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
