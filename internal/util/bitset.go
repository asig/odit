package util

type BitSet []uint64

func NewBitSet(size uint32) BitSet {
	return make(BitSet, (size+63)/64)
}

func (b BitSet) Set(bit uint32) {
	b[bit/64] |= 1 << (bit % 64)
}

func (b BitSet) Clear(bit uint32) {
	b[bit/64] &^= 1 << (bit % 64)
}

func (b BitSet) Test(bit uint32) bool {
	return b[bit/64]&(1<<(bit%64)) != 0
}
