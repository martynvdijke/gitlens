package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHubBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r, 0)
	}))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	hub.Broadcast([]byte("hello"))

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(msg) != "hello" {
		t.Fatalf("expected 'hello', got '%s'", string(msg))
	}
}

func TestHubMultipleClients(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r, 0)
	}))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http")

	conn1, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial1 error: %v", err)
	}
	defer conn1.Close()

	conn2, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial2 error: %v", err)
	}
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	hub.Broadcast([]byte("broadcast"))

	_, msg1, err := conn1.ReadMessage()
	if err != nil {
		t.Fatalf("read1 error: %v", err)
	}
	if string(msg1) != "broadcast" {
		t.Fatalf("conn1 expected 'broadcast', got '%s'", string(msg1))
	}

	_, msg2, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("read2 error: %v", err)
	}
	if string(msg2) != "broadcast" {
		t.Fatalf("conn2 expected 'broadcast', got '%s'", string(msg2))
	}
}

func TestHubBroadcastToUser(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r, 0)
	}))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http")

	// Anonymous client (userID=0)
	connAnon, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial anon error: %v", err)
	}
	defer connAnon.Close()

	// User-specific clients via separate servers with different userIDs
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r, 42)
	}))
	defer srv2.Close()

	u2 := "ws" + strings.TrimPrefix(srv2.URL, "http")
	connUser, _, err := websocket.DefaultDialer.Dial(u2, nil)
	if err != nil {
		t.Fatalf("dial user error: %v", err)
	}
	defer connUser.Close()

	time.Sleep(100 * time.Millisecond)

	// Broadcast to user 42 - only connUser should receive it
	hub.BroadcastToUser(42, []byte("secret-for-42"))

	// connUser should receive the message
	_, msg, err := connUser.ReadMessage()
	if err != nil {
		t.Fatalf("read user error: %v", err)
	}
	if string(msg) != "secret-for-42" {
		t.Fatalf("expected 'secret-for-42', got '%s'", string(msg))
	}

	// connAnon should NOT receive the message
	// Use a short timeout to confirm no message arrives
	connAnon.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, err = connAnon.ReadMessage()
	if err == nil {
		t.Fatal("anonymous client should not have received user-targeted broadcast")
	}
}

func TestHubClientDisconnect(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r, 0)
	}))
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	conn.Close()
	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Fatalf("expected 0 clients after disconnect, got %d", count)
	}
}
