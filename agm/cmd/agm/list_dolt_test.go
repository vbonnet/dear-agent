package main

import "testing"

func TestSessionListPaginationFlags(t *testing.T) {
	for name, defaultValue := range map[string]string{"limit": "1000", "offset": "0"} {
		flag := listCmdDolt.Flags().Lookup(name)
		if flag == nil {
			t.Fatalf("--%s flag is not registered", name)
		}
		if flag.DefValue != defaultValue {
			t.Errorf("--%s default = %q, want %q", name, flag.DefValue, defaultValue)
		}
	}
	flag := listCmdDolt.Flags().Lookup("stable-order")
	if flag == nil || flag.DefValue != "false" {
		t.Fatalf("--stable-order flag = %#v, want registered with false default", flag)
	}
}

func TestValidateListPagination(t *testing.T) {
	for _, test := range []struct {
		limit   int
		offset  int
		wantErr bool
	}{
		{limit: 1},
		{limit: 1000, offset: 1000},
		{limit: 0, wantErr: true},
		{limit: 1001, wantErr: true},
		{limit: 1000, offset: -1, wantErr: true},
	} {
		if err := validateListPagination(test.limit, test.offset); (err != nil) != test.wantErr {
			t.Errorf("validateListPagination(%d, %d) error = %v, wantErr %t", test.limit, test.offset, err, test.wantErr)
		}
	}
}
