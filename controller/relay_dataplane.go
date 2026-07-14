package controller

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/relayhub"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var relayUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsRelayConn adapts *websocket.Conn to relayhub.Conn with a write mutex.
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

	conn.SetReadLimit(8 * 1024 * 1024) // allow large result/file frames
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// Forward opaque frame to the peer; if absent, drop (client retries).
		_ = relayhub.Default.Forward(taskID, attempt, role, data)
	}
}
