package xair

import (
	"fmt"
	"slices"
)

func enumName(opts []string, i int32) (string, error) {
	if i < 0 || int(i) >= len(opts) {
		return "", fmt.Errorf("xair: enum index %d out of range (0..%d)", i, len(opts)-1)
	}
	return opts[i], nil
}

func enumIndex(opts []string, name string) (int, error) {
	i := slices.Index(opts, name)
	if i < 0 {
		return 0, fmt.Errorf("xair: %q is not one of %v", name, opts)
	}
	return i, nil
}
