package sszquery

// containerInfo maps a field's JSON name to its SSZ info (for nested Containers).
type containerInfo = map[string]*fieldInfo

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
