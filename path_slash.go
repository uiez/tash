package main

import (
	"path/filepath"
)

func stringToSlash(s string) string {
	return filepath.ToSlash(s)
}

func ptrsToSlash(ptr ...*string) {
	for _, ptr := range ptr {
		*ptr = filepath.ToSlash(*ptr)
	}
}

func sliceToSlash(paths []string) []string {
	for i := range paths {
		paths[i] = filepath.ToSlash(paths[i])
	}
	return paths
}
