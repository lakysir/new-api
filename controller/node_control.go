package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/nodehub"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// nodeControlUpgrader upgrades the Provider control channel. Browsers cannot set
// Authorization headers on a WebSocket, so the device token is passed via the
// Sec-WebSocket-Protocol header (subprotocol) — the same trick relay.go uses.
var nodeControlUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsNodeConn adapts *websocket.Conn to nodehub.NodeConn with a write mutex
// (gorilla forbids concurrent writers; the publisher and pong writer race).
type wsNodeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsNodeConn) SendJSON(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

func (w *wsNodeConn) Close() error { return w.conn.Close() }

// deviceTokenFromWS extracts the device access token from the subprotocol
// header. The plugin connects with subprotocols ["aitoken", "<deviceToken>"].
func deviceTokenFromWS(c *gin.Context) string {
	raw := c.Request.Header.Get("Sec-WebSocket-Protocol")
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	// Return the last non-"aitoken" protocol token as the credential.
	for i := len(parts) - 1; i >= 0; i-- {
		p := strings.TrimSpace(parts[i])
		if p != "" && p != "aitoken" {
			return p
		}
	}
	return ""
}

// HandleNodeControl is the Provider WSS control channel. It authenticates the
// device token, registers the node's live connection in the hub, updates
// presence on hello/heartbeat, and lets the Outbox publisher push task.offer.
func HandleNodeControl(c *gin.Context) {
	token := deviceTokenFromWS(c)
	device, err := model.AuthenticateDeviceByToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid device token"})
		return
	}

	// Echo the accepted subprotocol so the browser handshake completes.
	respHeader := http.Header{}
	if proto := c.Request.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		respHeader.Set("Sec-WebSocket-Protocol", "aitoken")
	}
	conn, err := nodeControlUpgrader.Upgrade(c.Writer, c.Request, respHeader)
	if err != nil {
		return // upgrade writes its own error
	}
	wrapped := &wsNodeConn{conn: conn}

	// The node id is provided by the plugin's node.hello; until then we key the
	// hub by device id as a fallback so presence still works.
	nodeID := device.Id
	nodehub.Default.Register(nodeID, wrapped)
	defer func() {
		nodehub.Default.Unregister(nodeID, wrapped)
		_ = conn.Close()
	}()

	conn.SetReadLimit(64 * 1024)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg map[string]any
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		typ, _ := msg["type"].(string)
		switch typ {
		case "node.hello", "node.resume":
			// Bind the real node id and register/refresh presence.
			if nid, ok := msg["node_id"].(string); ok && nid != "" {
				if nid != nodeID {
					nodehub.Default.Unregister(nodeID, wrapped)
					nodeID = nid
					nodehub.Default.Register(nodeID, wrapped)
				}
				region, _ := msg["region"].(string)
				version, _ := msg["version"].(string)
				_, _ = model.UpsertNode(device.UserId, device.Id, nodeID, region, version)
			}
			_ = wrapped.SendJSON(map[string]any{"type": "hello.ack", "device_id": device.Id})
		case "node.heartbeat":
			state, _ := msg["state"].(string)
			if state == "" {
				state = model.NodeStateIdle
			}
			// If the device was revoked while this socket stayed open, presence
			// is refused and we drop the connection so it can't keep the node
			// online (the plugin will fail to re-auth and re-register anew).
			if err := model.TouchNodePresence(nodeID, state); err != nil {
				if errors.Is(err, model.ErrNodeDeviceRevoked) {
					_ = wrapped.SendJSON(map[string]any{"type": "revoked"})
					return
				}
			}
			_ = wrapped.SendJSON(map[string]any{"type": "pong", "ts": time.Now().Unix()})
		case "task.accept", "task.reject", "task.result_ready", "task.failed", "receipt.submit":
			// Execution-result events are handled by the REST receipt endpoint in
			// the MVP; acknowledge to keep the channel flowing.
			_ = wrapped.SendJSON(map[string]any{"type": "ack", "event_id": msg["event_id"]})
		default:
			// ignore unknown control frames
		}
	}
}
