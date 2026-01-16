package gemini

import (
	"errors"
	"testing"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

func TestNewCommandTranslator(t *testing.T) {
	translator := NewCommandTranslator()
	if translator == nil {
		t.Fatal("NewCommandTranslator returned nil")
	}
}

func TestCommandTranslator_Translate(t *testing.T) {
	tests := []struct {
		name      string
		cmd       agent.Command
		wantType  string
		wantErr   error
		checkFunc func(*testing.T, GeminiOperation)
	}{
		{
			name: "rename success",
			cmd: agent.Command{
				Type:   agent.CommandRename,
				Params: map[string]interface{}{"name": "new-session-name"},
			},
			wantType: "rename_session",
			wantErr:  nil,
			checkFunc: func(t *testing.T, op GeminiOperation) {
				renameOp, ok := op.(*RenameSessionOperation)
				if !ok {
					t.Fatalf("expected *RenameSessionOperation, got %T", op)
				}
				if renameOp.NewName != "new-session-name" {
					t.Errorf("expected NewName='new-session-name', got %q", renameOp.NewName)
				}
			},
		},
		{
			name: "rename missing parameter",
			cmd: agent.Command{
				Type:   agent.CommandRename,
				Params: map[string]interface{}{},
			},
			wantErr: &ParameterError{
				CommandType:   agent.CommandRename,
				ParameterName: "name",
				Issue:         "missing",
			},
		},
		{
			name: "rename invalid type",
			cmd: agent.Command{
				Type:   agent.CommandRename,
				Params: map[string]interface{}{"name": 123},
			},
			wantErr: &ParameterError{
				CommandType:   agent.CommandRename,
				ParameterName: "name",
				Issue:         "must be a string",
			},
		},
		{
			name: "rename empty name",
			cmd: agent.Command{
				Type:   agent.CommandRename,
				Params: map[string]interface{}{"name": ""},
			},
			wantErr: &ParameterError{
				CommandType:   agent.CommandRename,
				ParameterName: "name",
				Issue:         "cannot be empty",
			},
		},
		{
			name: "setdir success",
			cmd: agent.Command{
				Type:   agent.CommandSetDir,
				Params: map[string]interface{}{"path": "/home/user/project"},
			},
			wantType: "set_directory",
			wantErr:  nil,
			checkFunc: func(t *testing.T, op GeminiOperation) {
				setDirOp, ok := op.(*SetDirectoryOperation)
				if !ok {
					t.Fatalf("expected *SetDirectoryOperation, got %T", op)
				}
				if setDirOp.Path != "/home/user/project" {
					t.Errorf("expected Path='/home/user/project', got %q", setDirOp.Path)
				}
			},
		},
		{
			name: "setdir missing parameter",
			cmd: agent.Command{
				Type:   agent.CommandSetDir,
				Params: map[string]interface{}{},
			},
			wantErr: &ParameterError{
				CommandType:   agent.CommandSetDir,
				ParameterName: "path",
				Issue:         "missing",
			},
		},
		{
			name: "setdir invalid type",
			cmd: agent.Command{
				Type:   agent.CommandSetDir,
				Params: map[string]interface{}{"path": 456},
			},
			wantErr: &ParameterError{
				CommandType:   agent.CommandSetDir,
				ParameterName: "path",
				Issue:         "must be a string",
			},
		},
		{
			name: "setdir empty path",
			cmd: agent.Command{
				Type:   agent.CommandSetDir,
				Params: map[string]interface{}{"path": ""},
			},
			wantErr: &ParameterError{
				CommandType:   agent.CommandSetDir,
				ParameterName: "path",
				Issue:         "cannot be empty",
			},
		},
		{
			name: "unsupported command - run hook",
			cmd: agent.Command{
				Type:   agent.CommandRunHook,
				Params: map[string]interface{}{"hook_name": "pre-commit"},
			},
			wantErr: ErrUnsupportedCommand,
		},
		{
			name: "unsupported command - authorize",
			cmd: agent.Command{
				Type:   agent.CommandAuthorize,
				Params: map[string]interface{}{"path": "/home/user"},
			},
			wantErr: ErrUnsupportedCommand,
		},
	}

	translator := NewCommandTranslator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op, err := translator.Translate(tt.cmd)

			// Check error
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}

				// Check if it's a ParameterError
				var paramErr *ParameterError
				if errors.As(tt.wantErr, &paramErr) {
					var gotParamErr *ParameterError
					if !errors.As(err, &gotParamErr) {
						t.Fatalf("expected ParameterError, got %T: %v", err, err)
					}
					if gotParamErr.CommandType != paramErr.CommandType {
						t.Errorf("expected CommandType=%v, got %v", paramErr.CommandType, gotParamErr.CommandType)
					}
					if gotParamErr.ParameterName != paramErr.ParameterName {
						t.Errorf("expected ParameterName=%v, got %v", paramErr.ParameterName, gotParamErr.ParameterName)
					}
					if gotParamErr.Issue != paramErr.Issue {
						t.Errorf("expected Issue=%v, got %v", paramErr.Issue, gotParamErr.Issue)
					}
				} else if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}

			// Check no error
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check operation type
			if op == nil {
				t.Fatal("expected operation, got nil")
			}
			if op.Type() != tt.wantType {
				t.Errorf("expected Type()=%q, got %q", tt.wantType, op.Type())
			}

			// Run custom check function
			if tt.checkFunc != nil {
				tt.checkFunc(t, op)
			}
		})
	}
}

func TestParameterError_Error(t *testing.T) {
	err := &ParameterError{
		CommandType:   agent.CommandRename,
		ParameterName: "name",
		Issue:         "missing",
	}
	expected := "command rename_session: parameter 'name': missing"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}

func TestRenameSessionOperation_Type(t *testing.T) {
	op := &RenameSessionOperation{NewName: "test"}
	if op.Type() != "rename_session" {
		t.Errorf("expected Type()='rename_session', got %q", op.Type())
	}
}

func TestRenameSessionOperation_Execute(t *testing.T) {
	op := &RenameSessionOperation{NewName: "test"}
	adapter := &GeminiAdapter{}
	err := op.Execute(adapter)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetDirectoryOperation_Type(t *testing.T) {
	op := &SetDirectoryOperation{Path: "/home/user"}
	if op.Type() != "set_directory" {
		t.Errorf("expected Type()='set_directory', got %q", op.Type())
	}
}

func TestSetDirectoryOperation_Execute(t *testing.T) {
	op := &SetDirectoryOperation{Path: "/home/user"}
	adapter := &GeminiAdapter{}
	err := op.Execute(adapter)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
