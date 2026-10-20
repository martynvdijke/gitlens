package ws

import (
	"net/http/httptest"
	"testing"
)

func TestCheckOriginAllowsSameHost(t *testing.T) {
	r := httptest.NewRequest("GET", "http://gitlens.local/ws", nil)
	r.Host = "gitlens.local"
	r.Header.Set("Origin", "https://gitlens.local")
	if !upgrader.CheckOrigin(r) {
		t.Fatal("expected same-host origin to be allowed")
	}
}

func TestCheckOriginRejectsCrossSite(t *testing.T) {
	r := httptest.NewRequest("GET", "http://gitlens.local/ws", nil)
	r.Host = "gitlens.local"
	r.Header.Set("Origin", "https://evil.example")
	if upgrader.CheckOrigin(r) {
		t.Fatal("expected cross-site origin to be rejected")
	}
}

func TestCheckOriginHandlesPorts(t *testing.T) {
	r := httptest.NewRequest("GET", "http://gitlens.local:6270/ws", nil)
	r.Host = "gitlens.local:6270"
	r.Header.Set("Origin", "http://gitlens.local:6270")
	if !upgrader.CheckOrigin(r) {
		t.Fatal("expected matching host:port origin to be allowed")
	}
}
