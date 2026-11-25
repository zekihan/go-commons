package utils

import (
	"github.com/tdewolff/minify/v2"

	commonsminify "github.com/zekihan/go-commons/minify"
)

// GetMinifier returns the shared minifier instance.
//
// Deprecated: use github.com/zekihan/go-commons/minify instead.
func GetMinifier() *minify.M {
	return commonsminify.GetMinifier()
}

// MinifyJS minifies JavaScript source.
//
// Deprecated: use github.com/zekihan/go-commons/minify instead.
func MinifyJS(input string) (string, error) {
	return commonsminify.MinifyJS(input) //nolint:wrapcheck // Thin re-export, return as-is.
}
