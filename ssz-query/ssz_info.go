package sszquery

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	ssz "github.com/prysmaticlabs/fastssz"
)

// sszInfo holds the pre-calculated SSZ data for a struct type.
type sszInfo struct {
	// Type of the SSZ structure (Basic, Container, List).
	sszType SSZType
	typ     reflect.Type

	// isVariable is true if the struct contains any variable-size fields.
	isVariable bool
	// fixedSize is the total size of the struct's fixed part.
	fixedSize uint64

	// For Container types:
	// fieldInfos maps a field's JSON name to its SSZ info (for nested Containers).
	fieldInfos map[string]*fieldInfo

	// For List/Vector types:
	elementInfo *sszInfo
}

type fieldInfo struct {
	sszInfo *sszInfo
	// offset is the offset of the field within the parent struct.
	offset uint64
}

func (info *sszInfo) FixedSize() uint64 {
	if info == nil {
		return 0
	}
	return info.fixedSize
}

func (info *sszInfo) UnmarshalFromSSZ(data []byte) (any, error) {
	if info == nil || info.typ == nil {
		return nil, fmt.Errorf("sszInfo or its type is nil")
	}

	newObjPtr := reflect.New(info.typ)

	unmarshaler, ok := newObjPtr.Interface().(ssz.Unmarshaler)
	if !ok {
		// If the type is `[]byte`, we can return the raw bytes directly.
		if info.typ.Kind() == reflect.Slice && info.typ.Elem().Kind() == reflect.Uint8 {
			return data, nil
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
		builder.WriteString(fmt.Sprintf("%s: %s (fixedSize: %d, isVariable: %t)\n", info.sszType, info.typ.Name(), info.fixedSize, info.isVariable))
	case List, Vector:
		builder.WriteString(fmt.Sprintf("%s[%s] (fixedSize: %d, isVariable: %t)\n", info.sszType, info.elementInfo.typ.Name(), info.fixedSize, info.isVariable))
	default:
		builder.WriteString(fmt.Sprintf("%s (fixedSize: %d, isVariable: %t)\n", info.sszType, info.fixedSize, info.isVariable))
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
