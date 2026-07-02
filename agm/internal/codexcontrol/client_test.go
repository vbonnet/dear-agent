package codexcontrol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadResponseSkipsNotifications(t *testing.T) {
	input := strings.NewReader(strings.Join([]string{
		`{"method":"thread/started","params":{"thread":{"id":"ignored"}}}`,
		`{"id":2,"result":{"thread":{"id":"thr_123","name":"agm-name","cwd":"/repo"}}}`,
	}, "\n"))

	var got struct {
		Thread Thread `json:"thread"`
	}
	if err := readResponse(json.NewDecoder(input), 2, &got); err != nil {
		t.Fatalf("readResponse returned error: %v", err)
	}
	if got.Thread.ID != "thr_123" {
		t.Fatalf("thread id = %q, want thr_123", got.Thread.ID)
	}
}

func TestReadResponseReturnsRPCError(t *testing.T) {
	input := strings.NewReader(`{"id":2,"error":{"code":-32602,"message":"bad params"}}`)

	err := readResponse(json.NewDecoder(input), 2, nil)
	if err == nil {
		t.Fatal("readResponse returned nil error")
	}
	if !strings.Contains(err.Error(), "bad params") {
		t.Fatalf("error = %q, want bad params", err.Error())
	}
}
