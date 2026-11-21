package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"edu-platform/backend/internal/config"
)

func TestProxyHeadersRequireExplicitTrust(t *testing.T) {
	for _, tc := range []struct {
		name, peer string
		trusted    []string
		want       string
	}{
		{"private peer is not automatically trusted", "10.0.0.7:1234", nil, "10.0.0.7"},
		{"loopback is not automatically trusted", "127.0.0.1:1234", nil, "127.0.0.1"},
		{"explicit proxy IP", "10.0.0.7:1234", []string{"10.0.0.7"}, "203.0.113.5"},
		{"explicit proxy subnet", "10.0.0.7:1234", []string{"10.0.0.0/24"}, "203.0.113.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{Config: &config.Config{TrustedProxyCIDRs: tc.trusted}}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.peer
			req.Header.Set("X-Real-IP", "203.0.113.5")
			app.realIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := clientIP(r); got != tc.want {
					t.Fatalf("client IP=%s want %s", got, tc.want)
				}
			})).ServeHTTP(httptest.NewRecorder(), req)
		})
	}
}
