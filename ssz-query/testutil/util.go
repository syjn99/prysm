package testutil

import (
	"fmt"

	ssz "github.com/ferranbt/fastssz"
)

func marshalAny(value any) ([]byte, error) {
	switch v := value.(type) {
	case ssz.Marshaler:
		return v.MarshalSSZ()
	case []byte:
		return v, nil
	case []uint64:
		println("Marshalling uint64 slice, length:", len(v))
		buf := make([]byte, len(v)*8)
		for i, val := range v {
			buf = ssz.MarshalUint64(buf[i*8:], val)
		}
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported type for SSZ marshalling: %T", value)
	}
}
