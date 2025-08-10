package testutil

import (
	"fmt"
	"reflect"

	ssz "github.com/ferranbt/fastssz"
)

func marshalAny(value any) ([]byte, error) {
	// First check if it implements ssz.Marshaler (this catches custom types like primitives.Epoch)
	if marshaler, ok := value.(ssz.Marshaler); ok {
		return marshaler.MarshalSSZ()
	}

	// Handle custom type aliases by checking if they're based on primitive types
	valueType := reflect.TypeOf(value)
	if valueType.PkgPath() != "" {
		switch valueType.Kind() {
		case reflect.Uint64:
			return ssz.MarshalUint64(make([]byte, 0), reflect.ValueOf(value).Uint()), nil
		case reflect.Uint32:
			return ssz.MarshalUint32(make([]byte, 0), uint32(reflect.ValueOf(value).Uint())), nil
		case reflect.Bool:
			return ssz.MarshalBool(make([]byte, 0), reflect.ValueOf(value).Bool()), nil
		}
	}

	switch v := value.(type) {
	case []byte:
		return v, nil
	case []uint64:
		buf := make([]byte, len(v)*8)
		for i, val := range v {
			buf = ssz.MarshalUint64(buf[i*8:], val)
		}
		return buf, nil
	case uint64:
		return ssz.MarshalUint64(make([]byte, 0), v), nil
	case bool:
		return ssz.MarshalBool(make([]byte, 0), v), nil

	default:
		return nil, fmt.Errorf("unsupported type for SSZ marshalling: %T", value)
	}
}
