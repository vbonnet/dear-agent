package restbackend

import "sync"

// ringBuffer is a fixed-size circular buffer for storing output lines.
type ringBuffer struct {
	mu    sync.Mutex
	items []string
	size  int
	head  int
	count int
}

func newRingBuffer(size int) *ringBuffer {
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
func (rb *ringBuffer) ReadLast(n int) []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

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
