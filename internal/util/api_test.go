package util

import "testing"

func TestUrlParse(t *testing.T) {
	testInside = true
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"localhost", "http://localhost:3000", "http://host.docker.internal:3000"},
		{"localhost", "http://localhost:3000/test", "http://host.docker.internal:3000/test"},
		{"localhost", "http://localhost:3123/test", "http://host.docker.internal:3123/test"},
		{"localhost", "https://api.agentuity.dev/test", "http://host.docker.internal:3012/test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := TransformUrl(test.url)
			if got != test.want {
				t.Errorf("TransformUrl(%q) = %q; want %q", test.url, got, test.want)
			}
		})
	}
}
