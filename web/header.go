package web

import (
	"mime"
	"net/http"
	"strconv"
)

type AcceptType string

const (
	AcceptTypeJSON  AcceptType = "json"
	AcceptTypeHTML  AcceptType = "html"
	AcceptTypePlain AcceptType = "plain"
	AcceptTypeNone  AcceptType = "none"
)

// GetAcceptType selects the supported explicit media type with the highest quality.
// Ties prefer JSON, then HTML, then plain text. Wildcards are not expanded.
func GetAcceptType(r *http.Request) AcceptType {
	qualities := make(map[string]float64)

	for _, header := range r.Header.Values("Accept") {
		for _, entry := range splitAcceptValues(header) {
			mediaType, params, err := mime.ParseMediaType(entry)
			if err != nil {
				continue
			}

			quality := 1.0
			if value, ok := params["q"]; ok {
				quality, err = strconv.ParseFloat(value, 64)
				if err != nil || !(quality > 0 && quality <= 1) {
					continue
				}
			}

			qualities[mediaType] = max(qualities[mediaType], quality)
		}
	}

	best := AcceptTypeNone
	bestQuality := 0.0

	for _, candidate := range []struct {
		mediaType  string
		acceptType AcceptType
	}{
		{"application/json", AcceptTypeJSON},
		{"text/html", AcceptTypeHTML},
		{"text/plain", AcceptTypePlain},
	} {
		if quality := qualities[candidate.mediaType]; quality > bestQuality {
			best = candidate.acceptType
			bestQuality = quality
		}
	}

	return best
}

// Split only at commas outside quoted parameter values.
func splitAcceptValues(header string) []string {
	var values []string

	quoted, escaped := false, false
	start := 0

	for index, char := range header {
		switch {
		case escaped:
			escaped = false
		case quoted && char == '\\':
			escaped = true
		case char == '"':
			quoted = !quoted
		case char == ',' && !quoted:
			values = append(values, header[start:index])
			start = index + 1
		default:
		}
	}

	return append(values, header[start:])
}
