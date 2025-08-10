package sszquery

type vectorInfo struct {
	// length is the length of the vector.
	length uint64
	// element is the SSZ info of the vector's element type.
	element *sszInfo
}

func (v *vectorInfo) Length() uint64 {
	if v == nil {
		return 0
	}
	return v.length
}

func (v *vectorInfo) Element() *sszInfo {
	if v == nil {
		return nil
	}
	return v.element
}
