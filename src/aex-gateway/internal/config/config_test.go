package config

import (
	"reflect"
	"testing"
)

func TestAllowedOriginsFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  string
		want []string
	}{
		{"unset", "", []string{"*"}},
		{"wildcard", "*", []string{"*"}},
		{"single", "https://app.example.com", []string{"https://app.example.com"}},
		{"list", "https://a.example.com,https://b.example.com", []string{"https://a.example.com", "https://b.example.com"}},
		{"whitespace", "  https://a.example.com , https://b.example.com  ", []string{"https://a.example.com", "https://b.example.com"}},
		{"empty entries dropped", "https://a.example.com,, ,https://b.example.com,", []string{"https://a.example.com", "https://b.example.com"}},
		{"trailing slash stripped", "https://ui.example.com/, https://b.example.com:8443/", []string{"https://ui.example.com", "https://b.example.com:8443"}},
		{"only separators", " , ,", []string{"*"}},
		{"only spaces", "   ", []string{"*"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("ALLOWED_ORIGINS", c.env)
			if got := Load().AllowedOrigins; !reflect.DeepEqual(got, c.want) {
				t.Errorf("AllowedOrigins = %q, want %q", got, c.want)
			}
		})
	}
}
