package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveHTTPAddr(t *testing.T) {
	// t.Setenv forbids t.Parallel in Go 1.22+.
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "default", want: ":8080"},
		{name: "bff_http_addr", env: map[string]string{"BFF_HTTP_ADDR": ":18080"}, want: ":18080"},
		{name: "bff_port", env: map[string]string{"BFF_PORT": "9090"}, want: ":9090"},
		{name: "port", env: map[string]string{"PORT": "7070"}, want: ":7070"},
		{
			name: "bff_http_addr_wins",
			env:  map[string]string{"BFF_HTTP_ADDR": ":1", "PORT": "2", "BFF_PORT": "3"},
			want: ":1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"BFF_HTTP_ADDR", "BFF_PORT", "PORT"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			require.Equal(t, tt.want, resolveHTTPAddr())
		})
	}
}
