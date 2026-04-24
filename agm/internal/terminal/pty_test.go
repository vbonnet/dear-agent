package terminal

import (
	"errors"
	"os/exec"
	"testing"
)

func TestMockPTY_StartWithPTY(t *testing.T) {
	tests := []struct {
		name     string
		startErr error
		wantErr  bool
	}{
		{
			name:     "successful start",
			startErr: nil,
			wantErr:  false,
		},
		{
			name:     "failed start",
			startErr: errors.New("start failed"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockPTY()
			mock.StartErr = tt.startErr

			cmd := exec.Command("true")
			err := mock.StartWithPTY(cmd)

			if (err != nil) != tt.wantErr {
				t.Errorf("StartWithPTY() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Clean up the process if it started
			if err == nil && cmd.Process != nil {
				cmd.Process.Kill()
				cmd.Wait()
			}
		})
	}
}

func TestMockPTY_Close(t *testing.T) {
	tests := []struct {
		name     string
		closeErr error
		wantErr  bool
	}{
		{
			name:     "successful close",
			closeErr: nil,
			wantErr:  false,
		},
		{
			name:     "failed close",
			closeErr: errors.New("close failed"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockPTY()
			mock.CloseErr = tt.closeErr

			err := mock.Close()

			if (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMockPTY_Read(t *testing.T) {
	tests := []struct {
		name     string
		readData []byte
		readErr  error
		bufSize  int
		wantN    int
		wantErr  bool
	}{
		{
			name:     "successful read",
			readData: []byte("hello world"),
			readErr:  nil,
			bufSize:  20,
			wantN:    11,
			wantErr:  false,
		},
		{
			name:     "read error",
			readData: nil,
			readErr:  errors.New("read failed"),
			bufSize:  20,
			wantN:    0,
			wantErr:  true,
		},
		{
			name:     "partial read",
			readData: []byte("hello world"),
			readErr:  nil,
			bufSize:  5,
			wantN:    5,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockPTY()
			mock.ReadData = tt.readData
			mock.ReadErr = tt.readErr

			buf := make([]byte, tt.bufSize)
			n, err := mock.Read(buf)

			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
			}
			if n != tt.wantN {
				t.Errorf("Read() n = %v, want %v", n, tt.wantN)
			}
		})
	}
}

func TestMockPTY_Write(t *testing.T) {
	tests := []struct {
		name     string
		writeErr error
		data     []byte
		wantN    int
		wantErr  bool
	}{
		{
			name:     "successful write",
			writeErr: nil,
			data:     []byte("test data"),
			wantN:    9,
			wantErr:  false,
		},
		{
			name:     "write error",
			writeErr: errors.New("write failed"),
			data:     []byte("test data"),
			wantN:    0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockPTY()
			mock.WriteErr = tt.writeErr

			n, err := mock.Write(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
			}
			if n != tt.wantN {
				t.Errorf("Write() n = %v, want %v", n, tt.wantN)
			}
			if !tt.wantErr && string(mock.Written) != string(tt.data) {
				t.Errorf("Write() written = %v, want %v", string(mock.Written), string(tt.data))
			}
		})
	}
}

func TestRealPTY_NewRealPTY(t *testing.T) {
	pty := NewRealPTY()
	if pty == nil {
		t.Error("NewRealPTY() returned nil")
	}
}

func TestRealPTY_Close_BeforeStart(t *testing.T) {
	pty := NewRealPTY()
	err := pty.Close()
	if err != nil {
		t.Errorf("Close() before Start() error = %v, want nil", err)
	}
}
