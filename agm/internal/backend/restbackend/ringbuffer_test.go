package restbackend

import "testing"

func TestRingBuffer_BoundsChecking(t *testing.T) {
	t.Run("zero size clamped to 1", func(t *testing.T) {
		rb := newRingBuffer(0)
		if rb.size != 1 {
			t.Errorf("expected size 1, got %d", rb.size)
		}
	})

	t.Run("negative size clamped to 1", func(t *testing.T) {
		rb := newRingBuffer(-100)
		if rb.size != 1 {
			t.Errorf("expected size 1, got %d", rb.size)
		}
	})

	t.Run("excessive size clamped to maxRingBufferSize", func(t *testing.T) {
		rb := newRingBuffer(maxRingBufferSize + 1)
		if rb.size != maxRingBufferSize {
			t.Errorf("expected size %d, got %d", maxRingBufferSize, rb.size)
		}
	})

	t.Run("valid size unchanged", func(t *testing.T) {
		rb := newRingBuffer(100)
		if rb.size != 100 {
			t.Errorf("expected size 100, got %d", rb.size)
		}
	})

	t.Run("ReadLast zero returns nil", func(t *testing.T) {
		rb := newRingBuffer(10)
		rb.Write("a")
		result := rb.ReadLast(0)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("ReadLast negative returns nil", func(t *testing.T) {
		rb := newRingBuffer(10)
		rb.Write("a")
		result := rb.ReadLast(-5)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("ReadLast more than stored returns all", func(t *testing.T) {
		rb := newRingBuffer(10)
		rb.Write("x")
		rb.Write("y")
		result := rb.ReadLast(1000)
		if len(result) != 2 {
			t.Errorf("expected 2 items, got %d", len(result))
		}
	})
}
