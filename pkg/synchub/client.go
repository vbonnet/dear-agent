package synchub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is a Go client for the synchub HTTP API. Surfaces (Discord bot,
// AGM session mirror, future Desktop RC bridge) embed this; tests can
// use it against a Server too.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClientFromTokenFile reads the token file written by the server and
// returns a configured Client. The path defaults to
// ~/.agm/sessions/<id>/synchub.token (see ServerOptions.TokenFile).
func NewClientFromTokenFile(path string) (*Client, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("synchub: read token file: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("synchub: malformed token file %q", path)
	}
	return &Client{
		BaseURL: "http://" + lines[0],
		Token:   lines[1],
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// AskQuestion -> POST /v1/qa/ask
func (c *Client) AskQuestion(ctx context.Context, prompt string) (QuestionID, error) {
	var resp askResp
	if err := c.do(ctx, "/v1/qa/ask", askReq{Prompt: prompt}, &resp); err != nil {
		return "", err
	}
	return resp.ID, nil
}

// Answer -> POST /v1/qa/answer. Returns the typed sentinel errors via
// errors.Is so callers can branch as if they were in-process.
func (c *Client) Answer(ctx context.Context, qid QuestionID, surface Surface, payload []byte) (Answer, error) {
	var resp answerResp
	if err := c.do(ctx, "/v1/qa/answer", answerReq{ID: qid, Surface: surface, Payload: payload}, &resp); err != nil {
		return Answer{}, err
	}
	return Answer{
		QuestionID: qid,
		Winner:     resp.Winner,
		Payload:    payload,
		At:         resp.At,
	}, nil
}

// Cancel -> POST /v1/qa/cancel.
func (c *Client) Cancel(ctx context.Context, qid QuestionID) error {
	var resp struct{}
	return c.do(ctx, "/v1/qa/cancel", cancelReq{ID: qid}, &resp)
}

// Acquire -> POST /v1/lock/acquire. The returned handle is server-side;
// the client just holds the fence token, and Release sends it back.
func (c *Client) Acquire(ctx context.Context, key, holder string, opts AcquireOptions) (*ClientLockHandle, error) {
	var resp acquireResp
	if err := c.do(ctx, "/v1/lock/acquire", acquireReq{
		Key: key, Holder: holder, Timeout: opts.Timeout, Deadline: opts.Deadline,
	}, &resp); err != nil {
		return nil, err
	}
	return &ClientLockHandle{
		client:   c,
		key:      key,
		holder:   holder,
		fence:    resp.Fence,
		acquired: resp.Acquired,
		deadline: resp.Deadline,
	}, nil
}

// ClientLockHandle mirrors LockHandle for remote callers.
type ClientLockHandle struct {
	client   *Client
	key      string
	holder   string
	fence    uint64
	acquired time.Time
	deadline time.Time
}

func (h *ClientLockHandle) Key() string         { return h.key }
func (h *ClientLockHandle) Holder() string      { return h.holder }
func (h *ClientLockHandle) Fence() uint64       { return h.fence }
func (h *ClientLockHandle) Deadline() time.Time { return h.deadline }

func (h *ClientLockHandle) Release(ctx context.Context) error {
	var resp struct{}
	return h.client.do(ctx, "/v1/lock/release", releaseReq{Key: h.key, Fence: h.fence}, &resp)
}

// do is the shared request/response shim. It maps server-side error
// codes back to the package's sentinel errors so client code reads
// identically to in-process code.
func (c *Client) do(ctx context.Context, path string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode >= 400 {
		var errBody struct {
			Code    string         `json:"code"`
			Error   string         `json:"error"`
			Details map[string]any `json:"details"`
		}
		_ = json.Unmarshal(respBody, &errBody)
		return clientErr(errBody.Code, errBody.Error, errBody.Details, httpResp.StatusCode)
	}
	if resp == nil {
		return nil
	}
	return json.Unmarshal(respBody, resp)
}

// clientErr maps a server error code to the package's sentinel so
// callers can errors.Is(err, ErrClosed) etc. The Details map (F-U3
// from multi-persona review) is preserved so surfaces can extract
// `winner`, `at_unix_ms`, etc. without parsing the error string.
func clientErr(code, message string, details map[string]any, status int) error {
	wrap := func(sentinel error) error {
		if message == "" {
			message = sentinel.Error()
		}
		return &RemoteError{code: code, message: message, sentinel: sentinel, status: status, details: details}
	}
	switch code {
	case "not_found":
		return wrap(ErrNotFound)
	case "closed":
		return wrap(ErrClosed)
	case "expired":
		return wrap(ErrExpired)
	case "already_held":
		return wrap(ErrAlreadyHeld)
	case "lock_timeout":
		return wrap(ErrLockTimeout)
	case "lock_closed":
		return wrap(ErrLockClosed)
	}
	return fmt.Errorf("synchub: server %d: %s", status, message)
}

// RemoteError is the wire-error type returned by Client calls. It
// satisfies errors.Is against the package's sentinels (so client code
// can branch identically to in-process code) and carries structured
// details so surfaces can reformat for humans.
type RemoteError struct {
	code     string
	message  string
	sentinel error
	status   int
	details  map[string]any
}

func (e *RemoteError) Error() string { return e.message }
func (e *RemoteError) Is(target error) bool {
	return target == e.sentinel || errors.Is(e.sentinel, target)
}
func (e *RemoteError) Unwrap() error          { return e.sentinel }
func (e *RemoteError) Code() string           { return e.code }
func (e *RemoteError) Details() map[string]any { return e.details }
