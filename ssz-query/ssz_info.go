package sszquery

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ssz "github.com/prysmaticlabs/fastssz"
)

// sszInfo holds the pre-calculated SSZ data for a struct type.
type sszInfo struct {
	// Type of the SSZ structure (Basic, Container, List).
	sszType SSZType
	// Type in Go. Need this for unmarshaling.
	typ reflect.Type

	// isVariable is true if the struct contains any variable-size fields.
	isVariable bool
	// fixedSize is the total size of the struct's fixed part.
	fixedSize uint64

	// For Container types:
	// fieldInfos maps a field's JSON name to its SSZ info (for nested Containers).
	fieldInfos map[string]*fieldInfo

	// For List types:
	listInfo *listInfo

	// For Vector types:
	vectorInfo *vectorInfo
}

type fieldInfo struct {
	// sszInfo contains the SSZ information of the field.
	sszInfo *sszInfo
	// offset is the offset of the field within the parent struct.
	offset uint64
	// actualOffset is the actual offset for variable-sized fields (read from marshalled data).
	actualOffset uint64
	// goFieldName is the name of the field in Go struct.
	goFieldName string
}

type listInfo struct {
	// limit is the maximum number of elements in the list.
	limit uint64
	// element is the SSZ info of the list's element type.
	element *sszInfo
	// length is the actual number of elements at runtime (0 if not set).
	length uint64
}

func (l *listInfo) Limit() uint64 {
	if l == nil {
		return 0
	}
	return l.limit
}

func (l *listInfo) ElementInfo() *sszInfo {
	if l == nil {
		return nil
	}
	return l.element
}

func (l *listInfo) Length() uint64 {
	if l == nil {
		return 0
	}
	return l.length
}

func (l *listInfo) SetLength(length uint64) error {
	if l == nil {
		return fmt.Errorf("listInfo is nil")
	}
	if length > l.limit {
		return fmt.Errorf("length %d exceeds limit %d", length, l.limit)
	}
	l.length = length
	return nil
}

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

func (v *vectorInfo) ElementInfo() *sszInfo {
	if v == nil {
		return nil
	}
	return v.element
}

func (info *sszInfo) FixedSize() uint64 {
	if info == nil {
		return 0
	}
	return info.fixedSize
}

func (info *sszInfo) ByteLength() uint64 {
	if info == nil {
		return 0
	}

	// Easy case: if the type is not variable, we can return the fixed size.
	if !info.isVariable {
		return info.fixedSize
	}

	switch info.sszType {
	case List:
		length := info.listInfo.length
		elementSize := info.listInfo.element.ByteLength()

		return length * elementSize

	case Container:
		size := info.fixedSize
		for _, fieldInfo := range info.fieldInfos {
			if !fieldInfo.sszInfo.isVariable {
				continue
			}

			size += fieldInfo.sszInfo.ByteLength()
		}
		return size

	default:
		return 0
	}
}

func (info *sszInfo) FieldInfos() (map[string]*fieldInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("sszInfo is nil")
	}

	if info.sszType != Container {
		return nil, fmt.Errorf("sszInfo is not a Container type, got %s", info.sszType)
	}

	if info.fieldInfos == nil {
		return nil, fmt.Errorf("sszInfo.fieldInfos is nil")
	}

	return info.fieldInfos, nil
}

func (info *sszInfo) ListInfo() (*listInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("sszInfo is nil")
	}

	if info.sszType != List {
		return nil, fmt.Errorf("sszInfo is not a List type, got %s", info.sszType)
	}

	if info.listInfo == nil {
		return nil, fmt.Errorf("sszInfo.listInfo is nil")
	}

	return info.listInfo, nil
}

func (info *sszInfo) VectorInfo() (*vectorInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("sszInfo is nil")
	}

	if info.sszType != Vector {
		return nil, fmt.Errorf("sszInfo is not a Vector type, got %s", info.sszType)
	}

	if info.vectorInfo == nil {
		return nil, fmt.Errorf("sszInfo.vectorInfo is nil")
	}

	return info.vectorInfo, nil
}

func (info *sszInfo) UnmarshalFromSSZ(data []byte) (any, error) {
	if info == nil || info.typ == nil {
		return nil, fmt.Errorf("sszInfo or its type is nil")
	}

	newObjPtr := reflect.New(info.typ)

	unmarshaler, ok := newObjPtr.Interface().(ssz.Unmarshaler)
	if !ok {
		// TODO: Remove ad-hoc check.

		// If the type is `[]byte`, we can return the raw bytes directly.
		if info.typ.Kind() == reflect.Slice && info.typ.Elem().Kind() == reflect.Uint8 {
			return data, nil
		}

		// Handle []uint64 type
		if info.typ.Kind() == reflect.Slice && info.typ.Elem().Kind() == reflect.Uint64 {
			if len(data)%8 != 0 {
				return nil, fmt.Errorf("invalid data length for []uint64: %d", len(data))
			}
			count := len(data) / 8
			result := make([]uint64, count)
			for i := 0; i < count; i++ {
				result[i] = bytesutil.FromBytes4(data[i*8 : i*8+4])
			}
			return result, nil
		}

		return nil, fmt.Errorf("type %v does not implement ssz.Unmarshaler", info.typ)
	}

	if err := unmarshaler.UnmarshalSSZ(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal for type %v: %w", info.typ, err)
	}

	return newObjPtr.Interface(), nil
}

func (info *sszInfo) Print() string {
	if info == nil {
		return "<nil>"
	}
	var builder strings.Builder
	printRecursive(info, &builder, "")
	return builder.String()
}

func printRecursive(info *sszInfo, builder *strings.Builder, prefix string) {
	switch info.sszType {
	case Container:
		builder.WriteString(fmt.Sprintf("%s: %s (size: %d, isVariable: %t)\n", info.sszType, info.typ.Name(), info.ByteLength(), info.isVariable))
	case List:
		builder.WriteString(fmt.Sprintf("%s[%s] (limit: %d, size: %d, isVariable: %t)\n", info.sszType, info.listInfo.element.typ.Name(), info.listInfo.limit, info.ByteLength(), info.isVariable))
	case Vector:
		builder.WriteString(fmt.Sprintf("%s[%s] (length: %d, size: %d, isVariable: %t)\n", info.sszType, info.vectorInfo.element.typ.Name(), info.vectorInfo.length, info.ByteLength(), info.isVariable))
	default:
		builder.WriteString(fmt.Sprintf("%s (size: %d, isVariable: %t)\n", info.sszType, info.ByteLength(), info.isVariable))
	}

	keys := make([]string, 0, len(info.fieldInfos))
	for k := range info.fieldInfos {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, key := range keys {
		connector := "├─"
		nextPrefix := prefix + "│  "
		if i == len(keys)-1 {
			connector = "└─"
			nextPrefix = prefix + "   "
		}

		builder.WriteString(fmt.Sprintf("%s%s %s (offset: %d) ", prefix, connector, key, info.fieldInfos[key].offset))

		if nestedInfo := info.fieldInfos[key].sszInfo; nestedInfo != nil {
			printRecursive(nestedInfo, builder, nextPrefix)
		} else {
			builder.WriteString("\n")
		}
	}
}
