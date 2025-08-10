package sszquery

import (
	"fmt"
	"strings"
)

// NOTE: Current `PathElement` only supports field names.
//
// TODO 1: Add feature for accessing by index of a list or vector.
// TODO 2: Add feature for getting the length of a list or vector.
type PathElement struct {
	Name string
}

func ParsePath(rawPath string) ([]PathElement, error) {
	// We use Dot notation, so we split the path by '.'.
	rawElements := strings.Split(rawPath, ".")
	if len(rawElements) == 0 {
		return nil, fmt.Errorf("empty path provided")
	}

	if rawElements[0] == "" {
		rawElements = rawElements[1:] // Remove leading dot if present
	}

	var path []PathElement
	for _, elem := range rawElements {
		path = append(path, PathElement{Name: elem})
	}

	return path, nil
}
