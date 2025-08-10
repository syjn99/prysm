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
	// Type of the SSZ structure (Basic, Container, List, etc.).
	sszType SSZType
	// Type in Go. Need this for unmarshaling.
	typ reflect.Type

	// isVariable is true if the struct contains any variable-size fields.
	isVariable bool
	// fixedSize is the total size of the struct's fixed part.
	fixedSize uint64

	// For Container types:
	containerInfo containerInfo

	// For List types:
	listInfo *listInfo

	// For Vector types:
	vectorInfo *vectorInfo
}

func (info *sszInfo) FixedSize() uint64 {
	if info == nil {
		return 0
	}
	return info.fixedSize
}

func (info *sszInfo) Size() uint64 {
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
		elementSize := info.listInfo.element.Size()

		return length * elementSize

	case Container:
		size := info.fixedSize
		for _, fieldInfo := range info.containerInfo {
			if !fieldInfo.sszInfo.isVariable {
				continue
			}

			size += fieldInfo.sszInfo.Size()
		}
		return size

	default:
		return 0
	}
}

func (info *sszInfo) ContainerInfo() (containerInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("sszInfo is nil")
	}

	if info.sszType != Container {
		return nil, fmt.Errorf("sszInfo is not a Container type, got %s", info.sszType)
	}

	if info.containerInfo == nil {
		return nil, fmt.Errorf("sszInfo.fieldInfos is nil")
	}

	return info.containerInfo, nil
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

	switch info.sszType {
	case UintN:
		converter := bytesutil.FromBytes8
		switch info.FixedSize() {
		case 1:
			converter = func(b []byte) uint64 {
				if len(b) != 1 {
					return 0
				}
				return uint64(b[0])
			}
		case 2:
			converter = func(b []byte) uint64 {
				if len(b) != 2 {
					return 0
				}
				return uint64(bytesutil.FromBytes2(b))
			}

		case 4:
			converter = bytesutil.FromBytes4
		case 8:
			converter = bytesutil.FromBytes8
		default:
			return nil, fmt.Errorf("unsupported UintN size: %d", info.FixedSize())
		}

		value := converter(data)

		// Check if this is a custom type alias (not a built-in type)
		if info.typ.PkgPath() != "" {
			return reflect.ValueOf(value).Convert(info.typ).Interface(), nil
		}

		return value, nil

	case Boolean:
		if len(data) != 1 {
			return nil, fmt.Errorf("invalid data length for bool: %d", len(data))
		}
		return data[0] != 0, nil

	default:
	}

	result := reflect.New(info.typ)
	unmarshaler, ok := result.Interface().(ssz.Unmarshaler)
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

	return result.Interface(), nil
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
		builder.WriteString(fmt.Sprintf("%s: %s (size: %d, isVariable: %t)\n", info.sszType, info.typ.Name(), info.Size(), info.isVariable))
	case List:
		builder.WriteString(fmt.Sprintf("%s[%s] (limit: %d, size: %d, isVariable: %t)\n", info.sszType, info.listInfo.element.typ.Name(), info.listInfo.limit, info.Size(), info.isVariable))
	case Vector:
		builder.WriteString(fmt.Sprintf("%s[%s] (length: %d, size: %d, isVariable: %t)\n", info.sszType, info.vectorInfo.element.typ.Name(), info.vectorInfo.length, info.Size(), info.isVariable))
	default:
		builder.WriteString(fmt.Sprintf("%s (size: %d, isVariable: %t)\n", info.sszType, info.Size(), info.isVariable))
	}

	keys := make([]string, 0, len(info.containerInfo))
	for k := range info.containerInfo {
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

		builder.WriteString(fmt.Sprintf("%s%s %s (offset: %d) ", prefix, connector, key, info.containerInfo[key].offset))

		if nestedInfo := info.containerInfo[key].sszInfo; nestedInfo != nil {
			printRecursive(nestedInfo, builder, nextPrefix)
		} else {
			builder.WriteString("\n")
		}
	}
}
