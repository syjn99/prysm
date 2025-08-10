package sszquery

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	sszMaxTag  = "ssz-max" // Used for variable-sized types like List and Bitlist.
	sszSizeTag = "ssz-size"
)

func PreCalculateSSZInfo(obj any) (*sszInfo, error) {
	// Get the value of the object using reflection.
	currentValue := reflect.ValueOf(obj)
	if currentValue.Kind() == reflect.Ptr {
		if currentValue.IsNil() {
			// If we encounter a nil pointer before the end of the path, we can still proceed
			// by analyzing the type, not the value.
			currentValue = reflect.New(currentValue.Type().Elem()).Elem()
		} else {
			currentValue = currentValue.Elem()
		}
	}

	info, err := analyzeType(currentValue.Type(), nil)
	if err != nil {
		return nil, fmt.Errorf("analyze type %s: %w", currentValue.Type().Name(), err)
	}

	return info, nil
}

func CalculateOffsetAndLength(sszInfo *sszInfo, path []PathElement) (*sszInfo, uint64, uint64, error) {
	if sszInfo == nil {
		return nil, 0, 0, fmt.Errorf("sszInfo is nil")
	}

	if len(path) == 0 {
		return nil, 0, 0, fmt.Errorf("path is empty")
	}

	walk := sszInfo
	currentOffset := uint64(0)

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
		walk = fieldInfo.sszInfo
	}

	// TODO: Handle variable-sized types.
	if walk.isVariable {
		return nil, 0, 0, fmt.Errorf("cannot calculate offset and length for variable-sized type %s", walk.typ.Name())
	}

	return walk, currentOffset, walk.FixedSize(), nil
}

// analyzeType is an entry point that inspects a reflect.Type and computes its SSZ layout information.
func analyzeType(typ reflect.Type, tag *reflect.StructTag) (*sszInfo, error) {
	switch typ.Kind() {
	// Basic types (e.g., uintN where N is 8, 16, 32, 64)
	// NOTE: uint128 and uint256 are represented as []byte in Go,
	// so we handle them as slices. See the case below.
	case reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Bool:
		return analyzeBasicType(typ)

	case reflect.Slice, reflect.Array:
		elemType := typ.Elem()
		// Special handling for byte slices.
		// e.g., Root (Bytes32), Signature (Bytes96)
		// e.g2., uint128 ([]bytes with length 16) and uint256 ([]byte with length 32).
		if elemType.Kind() == reflect.Uint8 {
			sszSize := tag.Get(sszSizeTag)
			if sszSize == "" {
				return nil, fmt.Errorf("ssz-size tag is required for byte slices")
			}

			byteLength, err := strconv.Atoi(sszSize)
			if err != nil {
				return nil, fmt.Errorf("invalid ssz-size tag for byte slice: %w",
					err)
			}

			return &sszInfo{
				// `BytesN` type is an alias of `Vector[byte, N]`, so we use Vector type.
				// TODO: How can we distinguish between `BytesN` and `uint{128,256}`?
				sszType: Vector,
				typ:     typ,

				fixedSize:  uint64(byteLength),
				isVariable: false,

				elementInfo: &sszInfo{
					sszType: UintN,
					typ:     elemType,

					fixedSize:  8,
					isVariable: false,
				},
			}, nil
		}

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
			sszInfo: info,
			offset:  currentOffset,
		}

		// Update the current offset based on the field's fixed size.
		currentOffset += info.fixedSize
	}

	sszInfo.fixedSize = currentOffset
	sszInfo.isVariable = structIsVariable
	return sszInfo, nil
}

// analyzeHomogeneousColType analyzes homogeneous collection types (e.g., List, Vector, Bitlist, Bitvector) and returns its SSZ info.
// TODO: We need to contain the element type.
func analyzeHomogeneousColType(typ reflect.Type, tag *reflect.StructTag) (*sszInfo, error) {
	if typ.Kind() != reflect.Slice && typ.Kind() != reflect.Array {
		return nil, fmt.Errorf("can only analyze slice/array types, got %v", typ.Kind())
	}

	if tag == nil {
		return nil, fmt.Errorf("tag is required for slice/array types")
	}

	// 1. Check if the type is List/Bitlist by checking `ssz-max` tag.
	sszMax := tag.Get(sszMaxTag)
	if sszMax != "" {
		limit, err := strconv.ParseUint(sszMax, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ssz-max tag (%s) on field: %w", sszMax, err)
		}

		return analyzeListType(typ, limit)
	}

	// 2. Handle Vector/Bitvector type.
	sszSize := tag.Get(sszSizeTag)
	dims := strings.Split(sszSize, ",")
	sizeVal, err := strconv.Atoi(dims[0])
	if err != nil {
		return nil, fmt.Errorf("invalid ssz-size tag (%s) on field: %w", sszSize, err)
	}

	return &sszInfo{
		// TODO: How do we distinguish between Vector and List?
		sszType: Vector,
		typ:     typ,

		fixedSize:  uint64(sizeVal),
		isVariable: false,
	}, nil
}

func analyzeListType(typ reflect.Type, _limit uint64) (*sszInfo, error) {
	elementInfo, err := analyzeType(typ.Elem(), nil)
	if err != nil {
		return nil, fmt.Errorf("analyze element type for List: %w", err)
	}

	return &sszInfo{
		// TODO: How do we distinguish between List and Bitlist?
		sszType: List,
		typ:     typ,

		fixedSize:  4,
		isVariable: true,

		elementInfo: elementInfo,
	}, nil
}
