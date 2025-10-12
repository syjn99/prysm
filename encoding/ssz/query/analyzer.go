package query

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

const offsetBytes = 4

// AnalyzeObject analyzes given object and returns its SSZ information.
func AnalyzeObject(obj SSZObject) (*sszInfo, error) {
	// Get the reflect.Value of the original object (which might be a pointer)
	objValue := reflect.ValueOf(obj)

	// Get the dereferenced value for type analysis
	derefValue := dereferencePointer(obj)

	// Pass both type AND value to analyzeType.
	// IMPORTANT: We pass objValue (which preserves the pointer) so that
	// analyzeContainerType can properly set the source field from the pointer type.
	info, err := analyzeType(derefValue.Type(), objValue, nil)
	if err != nil {
		return nil, fmt.Errorf("could not analyze type %s: %w", derefValue.Type().Name(), err)
	}

	// Populate variable-length information using the dereferenced value.
	err = PopulateVariableLengthInfo(info, derefValue.Interface())
	if err != nil {
		return nil, fmt.Errorf("could not populate variable length info: %w", err)
	}

	return info, nil
}

// PopulateVariableLengthInfo populates runtime information for SSZ fields of variable-sized types.
// This function updates the sszInfo structure with actual lengths and offsets that can only
// be determined at runtime for variable-sized items like Lists and variable-sized Container fields.
func PopulateVariableLengthInfo(sszInfo *sszInfo, value any) error {
	if sszInfo == nil {
		return errors.New("sszInfo is nil")
	}

	if value == nil {
		return errors.New("value is nil")
	}

	// Short circuit: If the type is fixed-sized, we don't need to fill in the info.
	if !sszInfo.isVariable {
		return nil
	}

	switch sszInfo.sszType {
	// In List case, we have to set the actual length of the list.
	case List:
		listInfo, err := sszInfo.ListInfo()
		if err != nil {
			return fmt.Errorf("could not get list info: %w", err)
		}

		if listInfo == nil {
			return errors.New("listInfo is nil")
		}

		val := reflect.ValueOf(value)
		if val.Kind() != reflect.Slice {
			return fmt.Errorf("expected slice for List type, got %v", val.Kind())
		}
		length := val.Len()

		if listInfo.element.isVariable {
			listInfo.elementSizes = make([]uint64, 0, length)

			// Populate nested variable-sized type element lengths recursively.
			for i := range length {
				if err := PopulateVariableLengthInfo(listInfo.element, val.Index(i).Interface()); err != nil {
					return fmt.Errorf("could not populate nested list element at index %d: %w", i, err)
				}
				listInfo.elementSizes = append(listInfo.elementSizes, listInfo.element.Size())
			}
		}

		if err := listInfo.SetLength(uint64(length)); err != nil {
			return fmt.Errorf("could not set list length: %w", err)
		}

		return nil

	// In Bitlist case, we have to set the actual length of the bitlist.
	case Bitlist:
		bitlistInfo, err := sszInfo.BitlistInfo()
		if err != nil {
			return fmt.Errorf("could not get bitlist info: %w", err)
		}

		if bitlistInfo == nil {
			return errors.New("bitlistInfo is nil")
		}

		val := reflect.ValueOf(value)
		if err := bitlistInfo.SetLengthFromBytes(val.Bytes()); err != nil {
			return fmt.Errorf("could not set bitlist length from bytes: %w", err)
		}

		return nil

	// In Container case, we need to recursively populate variable-sized fields.
	case Container:
		containerInfo, err := sszInfo.ContainerInfo()
		if err != nil {
			return fmt.Errorf("could not get container info: %w", err)
		}

		// Dereference first in case value is a pointer.
		derefValue := dereferencePointer(value)

		// Update source if it wasn't set during initial analysis.
		// This can happen if the container was created with a zero value initially.
		if derefValue.IsValid() && !derefValue.IsZero() && derefValue.CanInterface() {
			if sszObj, ok := derefValue.Interface().(SSZObject); ok {
				sszInfo.source = sszObj
			}
		}

		// Start with the fixed size of this Container.
		currentOffset := sszInfo.FixedSize()

		for _, fieldName := range containerInfo.order {
			fieldInfo := containerInfo.fields[fieldName]
			childSszInfo := fieldInfo.sszInfo
			if childSszInfo == nil {
				return fmt.Errorf("sszInfo is nil for field %s", fieldName)
			}

			// Get the field value for potential source setting on nested containers.
			fieldValue := derefValue.FieldByName(fieldInfo.goFieldName)

			// If this is a nested Container, ensure its source is set.
			if childSszInfo.sszType == Container {
				if fieldValue.IsValid() && !fieldValue.IsZero() && fieldValue.CanInterface() {
					// Try the value as-is (might be a pointer field)
					if sszObj, ok := fieldValue.Interface().(SSZObject); ok {
						childSszInfo.source = sszObj
					} else if fieldValue.Kind() == reflect.Struct && fieldValue.CanAddr() {
						// If it's a struct value and addressable, try getting its address
						if sszObj, ok := fieldValue.Addr().Interface().(SSZObject); ok {
							childSszInfo.source = sszObj
						}
					}
				}
			}

			// Skip fixed-size fields for offset calculation.
			if !childSszInfo.isVariable {
				continue
			}

			// Recursively populate variable-sized fields.
			if err := PopulateVariableLengthInfo(childSszInfo, fieldValue.Interface()); err != nil {
				return fmt.Errorf("could not populate from value for field %s: %w", fieldName, err)
			}

			// Each variable-sized element needs an offset entry.
			if childSszInfo.sszType == List {
				currentOffset += childSszInfo.listInfo.OffsetBytes()
			}

			// Set the actual offset for variable-sized fields.
			fieldInfo.offset = currentOffset

			currentOffset += childSszInfo.Size()
		}

		return nil
	default:
		return fmt.Errorf("unsupported SSZ type (%s) for variable size info", sszInfo.sszType)
	}
}

// analyzeType is an entry point that inspects a reflect.Type and computes its SSZ layout information.
// The value parameter is used to set the source field for Container types and propagate SSZObject references.
func analyzeType(typ reflect.Type, value reflect.Value, tag *reflect.StructTag) (*sszInfo, error) {
	switch typ.Kind() {
	// Basic types (e.g., uintN where N is 8, 16, 32, 64)
	// NOTE: uint128 and uint256 are represented as []byte in Go,
	// so we handle them as slices. See `analyzeHomogeneousColType`.
	case reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8, reflect.Bool:
		return analyzeBasicType(typ, value)

	case reflect.Slice:
		return analyzeHomogeneousColType(typ, value, tag)

	case reflect.Struct:
		return analyzeContainerType(typ, value)

	case reflect.Pointer:
		// Dereference pointer types.
		derefValue := value
		if value.IsValid() && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				// Create a zero value for type-only analysis
				fmt.Println("Dereferencing nil pointer for type:", typ.Elem().Name())
				derefValue = reflect.New(typ.Elem()).Elem()
			} else {
				derefValue = value.Elem()
			}
		}
		return analyzeType(typ.Elem(), derefValue, tag)

	default:
		return nil, fmt.Errorf("unsupported type %v for SSZ calculation", typ.Kind())
	}
}

// analyzeBasicType analyzes SSZ basic types (uintN, bool) and returns its info.
// Basic types don't implement SSZObject, so source is never set.
func analyzeBasicType(typ reflect.Type, value reflect.Value) (*sszInfo, error) {
	sszInfo := &sszInfo{
		typ: typ,

		// Every basic type is fixed-size and not variable.
		isVariable: false,
		// source is intentionally not set - basic types don't implement SSZObject
	}

	switch typ.Kind() {
	case reflect.Uint64:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 8
	case reflect.Uint32:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 4
	case reflect.Uint16:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 2
	case reflect.Uint8:
		sszInfo.sszType = UintN
		sszInfo.fixedSize = 1
	case reflect.Bool:
		sszInfo.sszType = Boolean
		sszInfo.fixedSize = 1
	default:
		return nil, fmt.Errorf("unsupported basic type %v for SSZ calculation", typ.Kind())
	}

	return sszInfo, nil
}

// analyzeHomogeneousColType analyzes homogeneous collection types (e.g., List, Vector, Bitlist, Bitvector) and returns its SSZ info.
// Collections themselves don't implement SSZObject, but their element type might (if it's a Container).
func analyzeHomogeneousColType(typ reflect.Type, value reflect.Value, tag *reflect.StructTag) (*sszInfo, error) {
	if typ.Kind() != reflect.Slice {
		return nil, fmt.Errorf("can only analyze slice types, got %v", typ.Kind())
	}

	// Parse the first dimension from the tag and get remaining tag for element
	sszDimension, remainingTag, err := ParseSSZTag(tag)
	if err != nil {
		return nil, fmt.Errorf("could not parse SSZ tag: %w", err)
	}
	if sszDimension == nil {
		return nil, errors.New("ssz tag is required for slice types")
	}

	// For element analysis, we analyze the element TYPE with a zero value.
	// Actual element values (for setting source on nested containers) are handled
	// during PopulateVariableLengthInfo when we iterate through the slice.
	elementType := typ.Elem()
	elementZeroValue := reflect.New(elementType).Elem()

	// Analyze element type with remaining dimensions
	elementInfo, err := analyzeType(elementType, elementZeroValue, remainingTag)
	if err != nil {
		return nil, fmt.Errorf("could not analyze element type for homogeneous collection: %w", err)
	}

	// 1. Handle List/Bitlist type
	if sszDimension.IsList() {
		limit, err := sszDimension.GetListLimit()
		if err != nil {
			return nil, fmt.Errorf("could not get list limit: %w", err)
		}

		return analyzeListType(typ, value, elementInfo, limit, sszDimension.isBitfield)
	}

	// 2. Handle Vector/Bitvector type
	if sszDimension.IsVector() {
		length, err := sszDimension.GetVectorLength()
		if err != nil {
			return nil, fmt.Errorf("could not get vector length: %w", err)
		}

		return analyzeVectorType(typ, value, elementInfo, length, sszDimension.isBitfield)
	}

	// Parsing ssz tag doesn't provide enough information to determine the collection type,
	// return an error.
	return nil, errors.New("could not determine collection type from tags")
}

// analyzeListType analyzes SSZ List/Bitlist type and returns its SSZ info.
// Lists/Bitlists don't implement SSZObject, so source is never set.
func analyzeListType(typ reflect.Type, value reflect.Value, elementInfo *sszInfo, limit uint64, isBitfield bool) (*sszInfo, error) {
	if isBitfield {
		return &sszInfo{
			sszType: Bitlist,
			typ:     typ,

			fixedSize:  offsetBytes,
			isVariable: true,

			bitlistInfo: &bitlistInfo{
				limit: limit,
			},
			// source is intentionally not set - slices don't implement SSZObject
		}, nil
	}

	if elementInfo == nil {
		return nil, errors.New("element info is required for List")
	}

	return &sszInfo{
		sszType: List,
		typ:     typ,

		fixedSize:  offsetBytes,
		isVariable: true,

		listInfo: &listInfo{
			limit:   limit,
			element: elementInfo,
		},
		// source is intentionally not set - slices don't implement SSZObject
	}, nil
}

// analyzeVectorType analyzes SSZ Vector/Bitvector type and returns its SSZ info.
// Vectors/Bitvectors don't implement SSZObject, so source is never set.
func analyzeVectorType(typ reflect.Type, value reflect.Value, elementInfo *sszInfo, length uint64, isBitfield bool) (*sszInfo, error) {
	if isBitfield {
		return &sszInfo{
			sszType: Bitvector,
			typ:     typ,

			// Size in bytes
			fixedSize:  length,
			isVariable: false,

			bitvectorInfo: &bitvectorInfo{
				length: length * 8, // length in bits
			},
			// source is intentionally not set - slices don't implement SSZObject
		}, nil
	}

	if elementInfo == nil {
		return nil, errors.New("element info is required for Vector/Bitvector")
	}

	// Validate the given length.
	// https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md#illegal-types
	if length == 0 {
		return nil, fmt.Errorf("vector length must be greater than 0, got %d", length)
	}

	return &sszInfo{
		sszType: Vector,
		typ:     typ,

		fixedSize:  length * elementInfo.Size(),
		isVariable: false,

		vectorInfo: &vectorInfo{
			length:  length,
			element: elementInfo,
		},
		// source is intentionally not set - slices don't implement SSZObject
	}, nil
}

// analyzeContainerType analyzes SSZ Container type and returns its SSZ info.
// This is where we set the source field if the value implements SSZObject.
func analyzeContainerType(typ reflect.Type, value reflect.Value) (*sszInfo, error) {
	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("can only analyze struct types, got %v", typ.Kind())
	}

	fields := make(map[string]*fieldInfo)
	order := make([]string, 0, typ.NumField())

	sszInfo := &sszInfo{
		sszType: Container,
		typ:     typ,
	}

	// Set source if value is valid and implements SSZObject.
	// This enables HashTreeRoot() to be called on this sszInfo.
	// Note: value might be a pointer (from top-level) or a struct value (from nested fields).
	// SSZObject methods are typically implemented on pointer receivers, so we need to handle both cases.
	if value.IsValid() && value.CanInterface() {
		// First try the value as-is (might already be a pointer)
		if sszObj, ok := value.Interface().(SSZObject); ok {
			sszInfo.source = sszObj
		} else if value.Kind() == reflect.Struct && value.CanAddr() {
			// If it's a struct value and addressable, try getting its address
			if sszObj, ok := value.Addr().Interface().(SSZObject); ok {
				sszInfo.source = sszObj
			}
		}
	}

	var currentOffset uint64

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Protobuf-generated structs contain private fields we must skip.
		// e.g., state, sizeCache, unknownFields, etc.
		if !field.IsExported() {
			continue
		}

		// The JSON tag contains the field name in the first part.
		// e.g., "attesting_indices,omitempty" -> "attesting_indices".
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			return nil, fmt.Errorf("field %s has no JSON tag", field.Name)
		}

		// NOTE: `fieldName` is a string with `snake_case` format (following consensus specs).
		fieldName := strings.Split(jsonTag, ",")[0]
		if fieldName == "" {
			return nil, fmt.Errorf("field %s has an empty JSON tag", field.Name)
		}

		// Get the field's value for nested analysis.
		var fieldValue reflect.Value
		if value.IsValid() && !value.IsZero() {
			// If value is a pointer, dereference it first to access fields
			actualValue := value
			if value.Kind() == reflect.Pointer {
				actualValue = value.Elem()
			}
			fieldValue = actualValue.FieldByName(field.Name)

			// For pointer fields (like *NestedContainer), we want to pass the pointer itself
			// so that nested containers can properly set their source field.
			// The analyzeType function will handle dereferencing in the Pointer case.
		} else {
			// Create zero value for type-only analysis.
			fieldValue = reflect.New(field.Type).Elem()
		}

		// Analyze each field so that we can complete full SSZ information.
		// Pass both the type AND value down for nested container source propagation.
		// IMPORTANT: For pointer fields, fieldValue will be the pointer, which is what we want
		// because SSZObject methods are implemented on pointer receivers.
		info, err := analyzeType(field.Type, fieldValue, &field.Tag)
		if err != nil {
			return nil, fmt.Errorf("could not analyze type for field %s: %w", fieldName, err)
		}

		// Store nested struct info.
		fields[fieldName] = &fieldInfo{
			sszInfo:     info,
			offset:      currentOffset,
			goFieldName: field.Name,
		}
		// Persist order
		order = append(order, fieldName)

		// Update the current offset depending on whether the field is variable-sized.
		if info.isVariable {
			// If one of the fields is variable-sized,
			// the entire struct is considered variable-sized.
			sszInfo.isVariable = true
			currentOffset += offsetBytes
		} else {
			currentOffset += info.fixedSize
		}
	}

	sszInfo.fixedSize = currentOffset
	sszInfo.containerInfo = &containerInfo{
		fields: fields,
		order:  order,
	}

	return sszInfo, nil
}

// dereferencePointer dereferences a pointer to get the underlying value using reflection.
func dereferencePointer(obj any) reflect.Value {
	value := reflect.ValueOf(obj)
	if value.Kind() == reflect.Pointer {
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
