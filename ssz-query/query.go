package sszquery

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ssz "github.com/ferranbt/fastssz"
)

const (
	sszMaxTag  = "ssz-max" // Used for variable-sized types like List and Bitlist.
	sszSizeTag = "ssz-size"
)

func DereferencePointer(obj any) reflect.Value {
	// Get the value of the object using reflection
	value := reflect.ValueOf(obj)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			// If we encounter a nil pointer before the end of the path, we can still proceed
			// by analyzing the type, not the value.
			value = reflect.New(value.Type().Elem()).Elem()
		} else {
			value = value.Elem()
		}
	}

	return value
}

func PreCalculateSSZInfo(obj any) (*sszInfo, error) {
	value := DereferencePointer(obj)

	info, err := analyzeType(value.Type(), nil)
	if err != nil {
		return nil, fmt.Errorf("analyze type %s: %w", value.Type().Name(), err)
	}

	return info, nil
}

func PopulateFromValue(sszInfo *sszInfo, value any) error {
	if sszInfo == nil {
		return fmt.Errorf("sszInfo is nil")
	}

	// If the type is not variable-sized, we don't need to fill in the info.
	if !sszInfo.isVariable {
		return nil
	}

	if value == nil {
		return fmt.Errorf("value is nil")
	}

	switch sszInfo.sszType {
	// In List case, we have to set the actual length of the list.
	case List:
		listInfo, err := sszInfo.ListInfo()
		if err != nil {
			return fmt.Errorf("get list info: %w", err)
		}

		val := reflect.ValueOf(value)
		if val.Kind() != reflect.Slice {
			return fmt.Errorf("expected slice for List type, got %v", val.Kind())
		}

		if err := listInfo.SetLength(uint64(val.Len())); err != nil {
			return fmt.Errorf("failed to set list length: %w", err)
		}

		return nil
	// In Container case, we need to recursively populate variable-sized fields.
	// Also, it is expected to read an actual offset from the marshalled data.
	case Container:
		marshalledData, err := value.(ssz.Marshaler).MarshalSSZ()
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}

		for fieldName, fieldInfo := range sszInfo.fieldInfos {
			childSszInfo := fieldInfo.sszInfo
			if childSszInfo == nil {
				return fmt.Errorf("sszInfo is nil for field %s", fieldName)
			}

			if !childSszInfo.isVariable {
				// Skip fixed-size fields.
				continue
			}

			if len(marshalledData) < int(fieldInfo.offset+4) {
				return fmt.Errorf("marshalled data is too short for field %s", fieldName)
			}

			// NOTE: The offset is always 4-byte sized.
			fieldInfo.actualOffset = bytesutil.FromBytes4(marshalledData[fieldInfo.offset : fieldInfo.offset+4])

			// Recursively populate variable-sized fields.
			fieldValue := DereferencePointer(value).FieldByName(fieldInfo.goFieldName)
			if err := PopulateFromValue(childSszInfo, fieldValue.Interface()); err != nil {
				return fmt.Errorf("populate from value for field %s: %w", fieldName, err)
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported SSZ type for variable size info: %s", sszInfo.sszType)
	}
}

func CalculateOffsetAndLength(sszInfo *sszInfo, path []PathElement) (*sszInfo, uint64, uint64, error) {
	if sszInfo == nil {
		return nil, 0, 0, fmt.Errorf("sszInfo is nil")
	}

	if len(path) == 0 {
		return nil, 0, 0, fmt.Errorf("path is empty")
	}

	walk := sszInfo
	actualOffset, currentOffset := uint64(0), uint64(0)

	for _, elem := range path {
		fieldInfos, err := walk.FieldInfos()
		if err != nil {
			// TODO: This logic is only for accessing the field in SSZ container types.
			return nil, 0, 0, fmt.Errorf("get field infos: %w", err)
		}

		fieldInfo, exists := fieldInfos[elem.Name]
		if !exists {
			return nil, 0, 0, fmt.Errorf("field %s not found in fieldInfos", elem.Name)
		}

		currentOffset += fieldInfo.offset
		actualOffset = fieldInfo.actualOffset
		walk = fieldInfo.sszInfo
	}

	offset := currentOffset
	if walk.isVariable {
		offset = actualOffset
	}

	return walk, offset, walk.ByteLength(), nil
}

// analyzeType is an entry point that inspects a reflect.Type and computes its SSZ layout information.
func analyzeType(typ reflect.Type, tag *reflect.StructTag) (*sszInfo, error) {
	switch typ.Kind() {
	// Basic types (e.g., uintN where N is 8, 16, 32, 64)
	// NOTE: uint128 and uint256 are represented as []byte in Go,
	// so we handle them as slices. See `analyzeHomogeneousColType`.
	case reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Bool:
		return analyzeBasicType(typ)

	case reflect.Slice:
		return analyzeHomogeneousColType(typ, tag)

	case reflect.Struct:
		return analyzeContainerType(typ)

	case reflect.Ptr:
		// Dereference pointer types.
		return analyzeType(typ.Elem(), tag)

	default:
		return nil, fmt.Errorf("unsupported type for SSZ calculation: %v", typ.Kind())
	}
}

// analyzeBasicType analyzes SSZ basic types (uintN, bool) and returns its info.
func analyzeBasicType(typ reflect.Type) (*sszInfo, error) {
	sszInfo := &sszInfo{
		typ: typ,

		// Every basic type is fixed-size and not variable.
		isVariable: false,
	}

	switch typ.Kind() {
	case reflect.Uint64:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 8
		return sszInfo, nil
	case reflect.Uint32:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 4
		return sszInfo, nil
	case reflect.Uint16:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 2
		return sszInfo, nil
	case reflect.Uint8:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 1
		return sszInfo, nil
	case reflect.Bool:
		sszInfo.sszType = Boolean
		sszInfo.fixedSize = 1
		return sszInfo, nil
	default:
		return nil, fmt.Errorf("unsupported basic type for SSZ calculation: %v", typ.Kind())
	}
}

// analyzeContainerType analyzes SSZ Container type and returns its SSZ info.
func analyzeContainerType(typ reflect.Type) (*sszInfo, error) {
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("can only analyze struct types, got %v", typ.Kind())
	}

	sszInfo := &sszInfo{
		sszType: Container,
		typ:     typ,

		fieldInfos: make(map[string]*fieldInfo),
	}
	var currentOffset uint64
	var structIsVariable bool

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Protobuf-generated structs contain private fields we must skip.
		// e.g., state, sizeCache, unknownFields, etc.
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			return nil, fmt.Errorf("field %s has no JSON tag", field.Name)
		}

		// The JSON tag contains the field name in the first part.
		// e.g., "attesting_indices,omitempty" -> "attesting_indices".
		// NOTE: `fieldName` is a string with `snake_case`` format (following consensus specs).
		fieldName := strings.Split(jsonTag, ",")[0]
		if fieldName == "" {
			return nil, fmt.Errorf("field %s has an empty JSON tag", field.Name)
		}

		// Analyze each field so that we can complete full SSZ information.
		info, err := analyzeType(field.Type, &field.Tag)
		if err != nil {
			return nil, fmt.Errorf("analyze type for field %s: %w", fieldName, err)
		}

		// If one of the fields is variable-sized,
		// the entire struct is considered variable-sized.
		if info.isVariable {
			structIsVariable = true
		}

		// Store nested struct info.
		sszInfo.fieldInfos[fieldName] = &fieldInfo{
			sszInfo:     info,
			offset:      currentOffset,
			goFieldName: field.Name,
		}

		// Update the current offset based on the field's fixed size.
		currentOffset += info.fixedSize
	}

	sszInfo.fixedSize = currentOffset
	sszInfo.isVariable = structIsVariable
	return sszInfo, nil
}

// analyzeHomogeneousColType analyzes homogeneous collection types (e.g., List, Vector, Bitlist, Bitvector) and returns its SSZ info.
func analyzeHomogeneousColType(typ reflect.Type, tag *reflect.StructTag) (*sszInfo, error) {
	if typ.Kind() != reflect.Slice {
		return nil, fmt.Errorf("can only analyze slice types, got %v", typ.Kind())
	}

	if tag == nil {
		return nil, fmt.Errorf("tag is required for slice types")
	}

	elementInfo, err := analyzeType(typ.Elem(), nil)
	if err != nil {
		return nil, fmt.Errorf("analyze element type for homogeneous collection: %w", err)
	}

	// 1. Check if the type is List/Bitlist by checking `ssz-max` tag.
	sszMax := tag.Get(sszMaxTag)
	if sszMax != "" {
		dims := strings.Split(sszMax, ",")
		limit, err := strconv.ParseUint(dims[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ssz-max tag (%s) on field: %w", sszMax, err)
		}

		return analyzeListType(typ, elementInfo, limit)
	}

	// 2. Handle Vector/Bitvector type.
	sszSize := tag.Get(sszSizeTag)
	dims := strings.Split(sszSize, ",")
	size, err := strconv.ParseUint(dims[0], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ssz-size tag (%s) on field: %w", sszSize, err)
	}

	return analyzeVectorType(typ, elementInfo, size)
}

// analyzeListType analyzes SSZ List type and returns its SSZ info.
func analyzeListType(typ reflect.Type, elementInfo *sszInfo, limit uint64) (*sszInfo, error) {
	if elementInfo == nil {
		return nil, fmt.Errorf("element info is required for List")
	}

	return &sszInfo{
		// TODO: How do we distinguish between List and Bitlist?
		sszType: List,
		typ:     typ,

		fixedSize:  4,
		isVariable: true,

		listInfo: &listInfo{
			limit:   limit,
			element: elementInfo,
			// NOTE: Length is not known until unmarshalling.
			// This will be set later in `PopulateFromValue`.
			length: 0,
		},
	}, nil
}

// analyzeVectorType analyzes SSZ Vector type and returns its SSZ info.
func analyzeVectorType(typ reflect.Type, elementInfo *sszInfo, length uint64) (*sszInfo, error) {
	if elementInfo == nil {
		return nil, fmt.Errorf("element info is required for Vector")
	}

	return &sszInfo{
		// TODO: How do we distinguish between Vector and Bitvector?
		sszType: Vector,
		typ:     typ,

		fixedSize:  length * elementInfo.FixedSize(),
		isVariable: false,

		vectorInfo: &vectorInfo{
			length:  length,
			element: elementInfo,
		},
	}, nil
}
