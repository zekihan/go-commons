package utils

import (
	"testing"
)

func TestMinifyJS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "simple JS",
			input:   "function add(a, b) { return a + b; }",
			want:    "function add(e,t){return e+t}",
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "invalid JS",
			input:   "function {",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MinifyJS(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MinifyJS() error = %v, wantErr %v", err, tt.wantErr)

				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("MinifyJS() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetMinifier_Singleton(t *testing.T) {
	m1 := GetMinifier()

	m2 := GetMinifier()
	if m1 != m2 {
		t.Error("GetMinifier() did not return the same instance")
	}
}
