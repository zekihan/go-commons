package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAcceptType(t *testing.T) {
	tests := []struct {
		name           string
		acceptHeader   string
		expectedResult AcceptType
	}{
		{"JSON Accept Header", "application/json", AcceptTypeJSON},
		{"HTML Accept Header", "text/html", AcceptTypeHTML},
		{"Plain Accept Header", "text/plain", AcceptTypePlain},
		{"No Accept Header", "", AcceptTypeNone},
		{"Unknown Accept Header", "application/xml", AcceptTypeNone},
		{"JSON prefix is not JSON", "application/jsonp", AcceptTypeNone},
		{"Media type in parameter", `application/xml; note="application/json"`, AcceptTypeNone},
		{"Comma in parameter", `application/xml; note="x,application/json,y"`, AcceptTypeNone},
		{"Quoted comma before HTML", `text/html; note="x,y"`, AcceptTypeHTML},
		{"Escaped quote before comma", `text/html; note="x\",y"`, AcceptTypeHTML},
		{"Case insensitive media type", "Application/JSON", AcceptTypeJSON},
		{"Excluded JSON", "application/json;q=0, text/html", AcceptTypeHTML},
		{"All excluded", "application/json;q=0, text/html;q=0", AcceptTypeNone},
		{"Higher quality HTML", "application/json;q=0.2, text/html;q=0.9", AcceptTypeHTML},
		{"Equal quality keeps JSON priority", "text/html, application/json", AcceptTypeJSON},
		{"Malformed quality", "application/json;q=invalid, text/plain", AcceptTypePlain},
		{"Out of range quality", "application/json;q=2, text/plain", AcceptTypePlain},
		{"Nonfinite quality", "application/json;q=NaN, text/plain", AcceptTypePlain},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}

			result := GetAcceptType(req)
			if result != tt.expectedResult {
				t.Errorf("expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestGetAcceptTypeMultipleHeaderValues(t *testing.T) {
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	req.Header.Add("Accept", "application/xml")
	req.Header.Add("Accept", "text/html")

	if got := GetAcceptType(req); got != AcceptTypeHTML {
		t.Errorf("expected HTML from second Accept header, got %v", got)
	}
}
