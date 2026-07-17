package controller

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/relayhub"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var relayUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	// relayPingInterval is how often the server pings each side to keep the
	// connection warm. It must stay comfortably below the shortest idle timeout
	// of any intermediary proxy (nginx proxy_read_timeout defaults to 60s), so
	// that a provider executing a long task never lets the relay go idle long
	// enough to be dropped before the result frame is delivered.
	relayPingInterval = 25 * time.Second
	// relayWriteWait bounds how long a single control-frame write may block.
	relayWriteWait = 10 * time.Second
	// relayPongWait is the read deadline; a missed pong past this window marks
	// the peer dead so the read loop exits instead of hanging forever. Browsers
	// (dashboard client and provider extension) auto-reply to pings with pongs.
	relayPongWait = 3 * relayPingInterval
)

// wsRelayConn adapts *websocket.Conn to relayhub.Conn with a write mutex. The
// mutex also serializes control (ping) writes against data-frame forwards.
type wsRelayConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsRelayConn) SendFrame(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(websocket.BinaryMessage, data)
}
func (w *wsRelayConn) Close() error { return w.conn.Close() }

// Ping sends a WebSocket ping control frame. Forwarding this through the proxy
// counts as upstream activity and resets its idle read timeout.
func (w *wsRelayConn) Ping() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(relayWriteWait))
}

// HandleDataPlaneRelay is the E2EE relay WSS endpoint. Each side (client /
// provider) connects with ?task_id=&attempt=&role=, authenticated by a device
// token (provider) or dashboard session/API key (client). The relay
// forwards opaque binary frames to the peer and never inspects or holds keys.
func HandleDataPlaneRelay(c *gin.Context) {
	taskID := c.Query("task_id")
	attempt, _ := strconv.Atoi(c.Query("attempt"))
	role := c.Query("role")
	if taskID == "" || (role != relayhub.RoleClient && role != relayhub.RoleProvider) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "task_id and valid role required"})
		return
	}

	// Authenticate by role (token travels in Sec-WebSocket-Protocol since
	// browsers can't set WS headers):
	//   provider → device access token (issued at activation);
	//   client   → dashboard login session, or an API key for SDK clients.
	token := deviceTokenFromWS(c)
	if role == relayhub.RoleProvider {
		if _, err := model.AuthenticateDeviceByToken(token); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid device token"})
			return
		}
	} else {
		order, err := model.GetOrder(taskID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "order not found"})
			return
		}
		clientID := 0
		if id, ok := sessions.Default(c).Get("id").(int); ok {
			clientID = id
		} else if apiToken, tokenErr := model.ValidateUserToken(token); tokenErr == nil && apiToken != nil {
			clientID = apiToken.UserId
		}
		if clientID == 0 || clientID != order.ClientId {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid client credentials"})
			return
		}
	}

	respHeader := http.Header{}
	if c.Request.Header.Get("Sec-WebSocket-Protocol") != "" {
		respHeader.Set("Sec-WebSocket-Protocol", "aitoken")
	}
	conn, err := relayUpgrader.Upgrade(c.Writer, c.Request, respHeader)
	if err != nil {
		return
	}
	wrapped := &wsRelayConn{conn: conn}
	relayhub.Default.Join(taskID, attempt, role, wrapped)
	defer func() {
		relayhub.Default.Leave(taskID, attempt, role, wrapped)
		_ = conn.Close()
	}()

	// Cap each frame at the shared data-plane limit. This is the authoritative
	// bandwidth guard: E2EE hides the content but the ciphertext still crosses
	// the relay, so an oversized config/result frame is dropped here (gorilla
	// sends a close frame and ends the read) regardless of what the client does.
	conn.SetReadLimit(relayhub.MaxFrameBytes)

	// Keepalive: an idle relay (provider busy executing, no frames flowing) must
	// not be dropped by a proxy idle timeout before the result is delivered. Ping
	// periodically and extend the read deadline on every pong / message.
	_ = conn.SetReadDeadline(time.Now().Add(relayPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(relayPongWait))
	})
	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		ticker := time.NewTicker(relayPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pingStop:
				return
			case <-ticker.C:
				if err := wrapped.Ping(); err != nil {
					// Peer gone; unblock the read loop by closing the connection.
					_ = conn.Close()
					return
				}
			}
		}
	}()

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// Any inbound traffic proves the peer is alive; extend the deadline.
		_ = conn.SetReadDeadline(time.Now().Add(relayPongWait))
		// Forward opaque frame to the peer; if absent, drop (client retries).
		_ = relayhub.Default.Forward(taskID, attempt, role, data)
	}
}
