package ws

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// clientConn associates a WebSocket connection with an authenticated user.
type clientConn struct {
	conn   *websocket.Conn
	userID int64
}

type Hub struct {
	clients    map[*websocket.Conn]*clientConn
	broadcast  chan []byte
	register   chan *clientConn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]*clientConn),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *clientConn, 64),
		unregister: make(chan *websocket.Conn, 64),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case cc := <-h.register:
			h.mu.Lock()
			h.clients[cc.conn] = cc
			h.mu.Unlock()
			log.Printf("WebSocket client connected (user=%d, total=%d)", cc.userID, len(h.clients))

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for _, cc := range h.clients {
				cc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := cc.conn.WriteMessage(websocket.TextMessage, message); err != nil {
					cc.conn.Close()
					delete(h.clients, cc.conn)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to every connected client.
func (h *Hub) Broadcast(msg []byte) {
	h.broadcast <- msg
}

// BroadcastToUser sends a message only to WebSocket connections belonging to the given user.
func (h *Hub) BroadcastToUser(userID int64, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, cc := range h.clients {
		if cc.userID == userID {
			cc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := cc.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				cc.conn.Close()
				delete(h.clients, cc.conn)
			}
		}
	}
}

// HandleWebSocket upgrades the connection and registers it with the given userID.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request, userID int64) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	h.register <- &clientConn{conn: conn, userID: userID}

	go func() {
		defer func() {
			h.unregister <- conn
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
