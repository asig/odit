package util

type BitSet []uint64

func NewBitSet(size int) BitSet {
	return make(BitSet, (size+63)/64)
}

func (b BitSet) Set(bit int) {
	b[bit/64] |= 1 << (bit % 64)
}

func (b BitSet) Clear(bit int) {
	b[bit/64] &^= 1 << (bit % 64)
}

func (b BitSet) Test(bit int) bool {
	return b[bit/64]&(1<<(bit%64)) != 0
}
