package restbackend

import "sync"

// maxRingBufferSize caps the per-stream buffer at 1 MiB worth of lines so
// a runaway producer can't exhaust process memory. The exact value isn't
// load-bearing — it just needs to be large enough to hold a few minutes
// of normal stdout while small enough that pathological inputs degrade
// gracefully.
const maxRingBufferSize = 1 << 20

// ringBuffer is a fixed-size circular buffer for storing output lines.
type ringBuffer struct {
	mu    sync.Mutex
	items []string
	size  int
	head  int
	count int
}

// newRingBuffer constructs a buffer of the requested size, clamped to
// the inclusive range [1, maxRingBufferSize]. Non-positive requests
// become 1; requests above the cap are reduced to the cap. This shape
// lets callers pass user-provided sizes without separately validating
// them — the buffer is always usable.
func newRingBuffer(size int) *ringBuffer {
	if size < 1 {
		size = 1
	}
	if size > maxRingBufferSize {
		size = maxRingBufferSize
	}
	return &ringBuffer{
		items: make([]string, size),
		size:  size,
	}
}

// Write appends a line to the buffer.
func (rb *ringBuffer) Write(s string) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.items[rb.head] = s
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

// ReadAll returns all stored lines in order (oldest first).
func (rb *ringBuffer) ReadAll() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	result := make([]string, rb.count)
	start := (rb.head - rb.count + rb.size) % rb.size
	for i := 0; i < rb.count; i++ {
		result[i] = rb.items[(start+i)%rb.size]
	}
	return result
}

// ReadLast returns the last n lines (or fewer if not enough stored).
// A non-positive n returns nil — callers asking for "no lines" don't
// want an empty slice they have to nil-check.
func (rb *ringBuffer) ReadLast(n int) []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if n <= 0 {
		return nil
	}
	if n > rb.count {
		n = rb.count
	}
	result := make([]string, n)
	start := (rb.head - n + rb.size) % rb.size
	for i := 0; i < n; i++ {
		result[i] = rb.items[(start+i)%rb.size]
	}
	return result
}
