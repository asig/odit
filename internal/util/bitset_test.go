package util

import (
	"testing"
)

func TestBitSet(t *testing.T) {
	bitset := NewBitSet(129)

	expected := []uint64{0, 0, 0}
	for i, v := range expected {
		if bitset[i] != v {
			t.Errorf("Expected bitset[%d] to be %d, got %d", i, v, bitset[i])
		}
	}

	bitset.Set(5)
	expected = []uint64{
		1 << 5, 0, 0,
	}
	for i, v := range expected {
		if bitset[i] != v {
			t.Errorf("Expected bitset[%d] to be %d, got %d", i, v, bitset[i])
		}
	}

	if !bitset.Test(5) {
		t.Errorf("Expected bit 5 to be set")
	}

	bitset.Clear(5)
	expected = []uint64{
		0, 0, 0,
	}
	for i, v := range expected {
		if bitset[i] != v {
			t.Errorf("Expected bitset[%d] to be %d, got %d", i, v, bitset[i])
		}
	}
	if bitset.Test(5) {
		t.Errorf("Expected bit 5 to be cleared")
	}
}
