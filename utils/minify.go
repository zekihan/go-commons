package utils

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/js"
)

var (
	minifier     *minify.M
	minifierOnce sync.Once
)

func GetMinifier() *minify.M {
	minifierOnce.Do(func() {
		minifier = minify.New()
		minifier.AddFuncRegexp(
			regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$"),
			js.Minify,
		)
	})

	return minifier
}

func MinifyJS(input string) (string, error) {
	m := GetMinifier()

	output, err := m.String("text/javascript", input)
	if err != nil {
		return "", fmt.Errorf("failed to minify JS: %w", err)
	}

	return output, nil
}
