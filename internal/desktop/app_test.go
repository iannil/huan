package desktop

import (
	"reflect"
	"testing"
)

func TestAdminOrigin(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want []string
	}{
		{"admin url with path", "http://127.0.0.1:61932/admin/", []string{"http://127.0.0.1:61932"}},
		{"base url without path", "http://127.0.0.1:61932/", []string{"http://127.0.0.1:61932"}},
		{"empty when no server", "", nil},
		{"empty when no host", "not-a-url", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := adminOrigin(tc.url)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("adminOrigin(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}
