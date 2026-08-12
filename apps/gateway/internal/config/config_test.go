package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveHTTPAddr(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "default", want: ":8080"},
		{name: "gateway_http_addr", env: map[string]string{"GATEWAY_HTTP_ADDR": ":18080"}, want: ":18080"},
		{name: "gateway_port", env: map[string]string{"GATEWAY_PORT": "9090"}, want: ":9090"},
		{name: "port", env: map[string]string{"PORT": "7070"}, want: ":7070"},
		{
			name: "gateway_http_addr_wins",
			env:  map[string]string{"GATEWAY_HTTP_ADDR": ":1", "PORT": "2", "GATEWAY_PORT": "3"},
			want: ":1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"GATEWAY_HTTP_ADDR", "GATEWAY_PORT", "PORT"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			require.Equal(t, tt.want, resolveHTTPAddr())
		})
	}
}

func TestLoadUpstream(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_ADDR", ":8080")
	t.Setenv("BFF_UPSTREAM", "")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8081", cfg.BFFUpstream)

	t.Setenv("BFF_UPSTREAM", "not-a-url")
	_, err = Load()
	require.Error(t, err)
}
