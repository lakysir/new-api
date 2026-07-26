package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service/nodeidentity"
	"gorm.io/gorm"
)

// Device lifecycle states.
const (
	DeviceStatusActive  = "active"
	DeviceStatusRevoked = "revoked"
)

// Default token lifetimes. Access tokens are short-lived; the device-bound
// refresh token is long-lived but revocable (architecture §6.6).
const (
	deviceAccessTokenTTL  = 30 * time.Minute
	deviceRefreshTokenTTL = 30 * 24 * time.Hour
	deviceChallengeTTL    = 5 * time.Minute
)

var (
	// ErrDeviceNotFound is returned when a device row is missing.
	ErrDeviceNotFound = errors.New("device not found")
	// ErrDeviceRevoked is returned when a revoked device is used.
	ErrDeviceRevoked = errors.New("device is revoked")
	// ErrChallengeInvalid is returned for a missing/expired/used challenge.
	ErrChallengeInvalid = errors.New("device challenge invalid or expired")
	// ErrDeviceTokenInvalid is returned when an access/refresh token does not match.
	ErrDeviceTokenInvalid = errors.New("device token invalid")
)

// DeviceChallenge is a one-time nonce a plugin must sign with its device key to
// prove key possession during activation. Consumed on first successful use.
type DeviceChallenge struct {
	Id         string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	UserId     int    `json:"user_id" gorm:"index;not null"`
	Nonce      string `json:"nonce" gorm:"type:varchar(128);not null"`
	ExpiresAt  int64  `json:"expires_at" gorm:"index;not null"`
	ConsumedAt int64  `json:"consumed_at" gorm:"default:0"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (DeviceChallenge) TableName() string { return "device_challenges" }

// Device is a registered Provider install instance with its own Ed25519 public
// key. Only token hashes are stored; raw tokens live only on the device.
type Device struct {
	Id               string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	UserId           int    `json:"user_id" gorm:"index;not null"`
	PublicKey        string `json:"public_key" gorm:"type:varchar(128);not null"` // base64 Ed25519
	Name             string `json:"name" gorm:"type:varchar(128)"`
	// Nickname is a user-editable label to tell devices apart in the console.
	// Distinct from Name, which the plugin reports at activation ("browser-node").
	Nickname string `json:"nickname" gorm:"type:varchar(128)"`
	Status   string `json:"status" gorm:"type:varchar(16);index;default:active"`
	AccessTokenHash  string `json:"-" gorm:"type:varchar(80);index"`
	AccessExpiresAt  int64  `json:"access_expires_at" gorm:"default:0"`
	RefreshTokenHash string `json:"-" gorm:"type:varchar(80);index"`
	RefreshExpiresAt int64  `json:"refresh_expires_at" gorm:"default:0"`
	LastSeenAt       int64  `json:"last_seen_at" gorm:"default:0"`
	RevokedAt        int64  `json:"revoked_at" gorm:"default:0"`
	CreatedAt        int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Device) TableName() string { return "devices" }

// DeviceSession is the token pair returned to the plugin on activate/refresh.
// Raw tokens are returned exactly once and never persisted in plaintext.
type DeviceSession struct {
	DeviceId         string `json:"device_id"`
	AccessToken      string `json:"access_token"`
	AccessExpiresAt  int64  `json:"access_expires_at"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresAt int64  `json:"refresh_expires_at"`
}

// CreateDeviceChallenge issues a fresh signing challenge for a user.
func CreateDeviceChallenge(userId int) (*DeviceChallenge, error) {
	nonce, err := nodeidentity.GenerateNonce(32)
	if err != nil {
		return nil, err
	}
	ch := &DeviceChallenge{
		Id:        "dch_" + common.GetUUID(),
		UserId:    userId,
		Nonce:     nonce,
		ExpiresAt: time.Now().Add(deviceChallengeTTL).Unix(),
	}
	if err := DB.Create(ch).Error; err != nil {
		return nil, err
	}
	return ch, nil
}

// issueTokens mints a new token pair and stamps hashes onto the device.
func (d *Device) issueTokens() (*DeviceSession, error) {
	accessTok, accessHash, err := nodeidentity.GenerateToken()
	if err != nil {
		return nil, err
	}
	refreshTok, refreshHash, err := nodeidentity.GenerateToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	d.AccessTokenHash = accessHash
	d.AccessExpiresAt = now.Add(deviceAccessTokenTTL).Unix()
	d.RefreshTokenHash = refreshHash
	d.RefreshExpiresAt = now.Add(deviceRefreshTokenTTL).Unix()
	return &DeviceSession{
		DeviceId:         d.Id,
		AccessToken:      accessTok,
		AccessExpiresAt:  d.AccessExpiresAt,
		RefreshToken:     refreshTok,
		RefreshExpiresAt: d.RefreshExpiresAt,
	}, nil
}

// ActivateDevice verifies a signed challenge and registers the device, atomically
// consuming the challenge so it cannot be replayed. It is idempotent on
// (user_id, public_key): re-activating with the same device key reuses the
// existing device row and reissues tokens instead of creating a duplicate, so a
// user clicking "register" repeatedly (or a browser restart re-registering)
// never spawns multiple devices for the same install.
func ActivateDevice(userId int, challengeId, publicKey, signature, name string) (*Device, *DeviceSession, error) {
	var device *Device
	var session *DeviceSession
	err := DB.Transaction(func(tx *gorm.DB) error {
		var ch DeviceChallenge
		if err := tx.Where("id = ? AND user_id = ?", challengeId, userId).First(&ch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChallengeInvalid
			}
			return err
		}
		if ch.ConsumedAt != 0 || ch.ExpiresAt < time.Now().Unix() {
			return ErrChallengeInvalid
		}
		if err := nodeidentity.VerifyChallengeSignature(publicKey, ch.Nonce, signature); err != nil {
			return err
		}
		// Consume the challenge (only if still unconsumed) to block replay.
		res := tx.Model(&DeviceChallenge{}).
			Where("id = ? AND consumed_at = 0", ch.Id).
			Update("consumed_at", time.Now().Unix())
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrChallengeInvalid
		}

		// Idempotency: reuse an existing active device with the same public key.
		var existing Device
		lookup := tx.Where("user_id = ? AND public_key = ?", userId, publicKey).First(&existing)
		if lookup.Error == nil {
			if existing.Status == DeviceStatusRevoked {
				return ErrDeviceRevoked
			}
			s, err := existing.issueTokens()
			if err != nil {
				return err
			}
			existing.LastSeenAt = time.Now().Unix()
			if err := tx.Model(&Device{}).Where("id = ?", existing.Id).Updates(map[string]any{
				"access_token_hash":  existing.AccessTokenHash,
				"access_expires_at":  existing.AccessExpiresAt,
				"refresh_token_hash": existing.RefreshTokenHash,
				"refresh_expires_at": existing.RefreshExpiresAt,
				"last_seen_at":       existing.LastSeenAt,
			}).Error; err != nil {
				return err
			}
			device, session = &existing, s
			return nil
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}

		d := &Device{
			Id:         "dev_" + common.GetUUID(),
			UserId:     userId,
			PublicKey:  publicKey,
			Name:       name,
			Status:     DeviceStatusActive,
			LastSeenAt: time.Now().Unix(),
		}
		s, err := d.issueTokens()
		if err != nil {
			return err
		}
		if err := tx.Create(d).Error; err != nil {
			return err
		}
		device, session = d, s
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return device, session, nil
}

// RefreshDeviceSession rotates the token pair given a valid refresh token.
func RefreshDeviceSession(deviceId, refreshToken string) (*DeviceSession, error) {
	var session *DeviceSession
	err := DB.Transaction(func(tx *gorm.DB) error {
		var d Device
		if err := tx.Where("id = ?", deviceId).First(&d).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDeviceNotFound
			}
			return err
		}
		if d.Status == DeviceStatusRevoked {
			return ErrDeviceRevoked
		}
		if d.RefreshTokenHash == "" || !nodeidentity.ConstantTimeEqual(d.RefreshTokenHash, nodeidentity.HashToken(refreshToken)) {
			return ErrDeviceTokenInvalid
		}
		if d.RefreshExpiresAt < time.Now().Unix() {
			return ErrDeviceTokenInvalid
		}
		s, err := d.issueTokens()
		if err != nil {
			return err
		}
		d.LastSeenAt = time.Now().Unix()
		if err := tx.Model(&Device{}).Where("id = ?", d.Id).Updates(map[string]any{
			"access_token_hash":  d.AccessTokenHash,
			"access_expires_at":  d.AccessExpiresAt,
			"refresh_token_hash": d.RefreshTokenHash,
			"refresh_expires_at": d.RefreshExpiresAt,
			"last_seen_at":       d.LastSeenAt,
		}).Error; err != nil {
			return err
		}
		session = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// AuthenticateDeviceAccessToken resolves an active device by access token,
// enforcing revocation and expiry. Used by the device auth middleware.
func AuthenticateDeviceAccessToken(deviceId, accessToken string) (*Device, error) {
	var d Device
	if err := DB.Where("id = ?", deviceId).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	if d.Status == DeviceStatusRevoked {
		return nil, ErrDeviceRevoked
	}
	if d.AccessTokenHash == "" || !nodeidentity.ConstantTimeEqual(d.AccessTokenHash, nodeidentity.HashToken(accessToken)) {
		return nil, ErrDeviceTokenInvalid
	}
	if d.AccessExpiresAt < time.Now().Unix() {
		return nil, ErrDeviceTokenInvalid
	}
	return &d, nil
}

// AuthenticateDeviceByToken resolves an active device from just the access
// token (no device id needed) by matching its stored hash. Used by the device
// auth middleware so the plugin only has to send `Authorization: Bearer
// <deviceAccessToken>`. Returns ErrDeviceTokenInvalid on any miss so callers
// never leak which part failed.
func AuthenticateDeviceByToken(accessToken string) (*Device, error) {
	if accessToken == "" {
		return nil, ErrDeviceTokenInvalid
	}
	var d Device
	err := DB.Where("access_token_hash = ?", nodeidentity.HashToken(accessToken)).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrDeviceTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	if d.Status == DeviceStatusRevoked {
		return nil, ErrDeviceRevoked
	}
	if d.AccessExpiresAt < time.Now().Unix() {
		return nil, ErrDeviceTokenInvalid
	}
	return &d, nil
}

// RevokeDevice marks a device revoked and clears its tokens so neither refresh
// nor access can succeed afterwards. It also suspends the device's nodes.
func RevokeDevice(userId int, deviceId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Device{}).
			Where("id = ? AND user_id = ?", deviceId, userId).
			Updates(map[string]any{
				"status":             DeviceStatusRevoked,
				"revoked_at":         common.GetTimestamp(),
				"access_token_hash":  "",
				"refresh_token_hash": "",
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrDeviceNotFound
		}
		// Take the device's nodes offline and suspend their capabilities.
		if err := tx.Model(&Node{}).Where("device_id = ?", deviceId).
			Update("state", NodeStateOffline).Error; err != nil {
			return err
		}
		return suspendCapabilitiesByDeviceTx(tx, deviceId, "device_revoked")
	})
}

// ListUserDevices returns a user's devices (no token hashes leak via json tag).
func ListUserDevices(userId int) ([]Device, error) {
	var devices []Device
	err := DB.Where("user_id = ?", userId).Order("created_at desc").Find(&devices).Error
	return devices, err
}

// ConsoleRow is one row of the provider console: a device plus the nodes that
// belong to it. Nodes with no surviving device row are returned as rows with a
// nil Device (the console renders them as standalone nodes).
type ConsoleRow struct {
	Device *Device `json:"device"`
	Nodes  []Node  `json:"nodes"`
}

// ListUserConsoleRows returns one page of the caller's console rows plus the
// total row count. Paging is done server-side so the console never loads (nor
// fans out per-node requests for) more than one page of devices/nodes at a
// time — a provider may own hundreds.
//
// Rows are ordered devices-first (newest device first), then device-less nodes,
// so a given (offset, limit) is stable across calls. When activeOnly is set,
// revoked devices and offline nodes are excluded entirely, matching the
// console's "hide revoked/offline" filter — the filter must be applied in SQL,
// otherwise a page could come back empty while later pages still hold matches.
func ListUserConsoleRows(userId int, activeOnly bool, offset, limit int) ([]ConsoleRow, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	onlineCutoff := time.Now().Add(-NodePresenceTimeout).Unix()

	deviceQuery := func() *gorm.DB {
		q := DB.Model(&Device{}).Where("user_id = ?", userId)
		if activeOnly {
			q = q.Where("status = ?", DeviceStatusActive)
		}
		return q
	}
	// Nodes whose device row no longer exists for this user. Kept visible so a
	// node can still be inspected/deleted after its device is purged.
	orphanNodeQuery := func() *gorm.DB {
		q := DB.Model(&Node{}).Where("user_id = ?", userId).
			Where("device_id NOT IN (?)",
				DB.Model(&Device{}).Select("id").Where("user_id = ?", userId))
		if activeOnly {
			q = q.Where("state <> ? AND last_seen_at >= ?", NodeStateOffline, onlineCutoff)
		}
		return q
	}

	var deviceTotal, orphanTotal int64
	if err := deviceQuery().Count(&deviceTotal).Error; err != nil {
		return nil, 0, err
	}
	if err := orphanNodeQuery().Count(&orphanTotal).Error; err != nil {
		return nil, 0, err
	}
	total := deviceTotal + orphanTotal

	rows := make([]ConsoleRow, 0, limit)

	// Slice the device segment of the combined sequence.
	if int64(offset) < deviceTotal {
		deviceLimit := limit
		if remaining := deviceTotal - int64(offset); remaining < int64(deviceLimit) {
			deviceLimit = int(remaining)
		}
		var devices []Device
		if err := deviceQuery().Order("created_at desc, id desc").
			Offset(offset).Limit(deviceLimit).Find(&devices).Error; err != nil {
			return nil, 0, err
		}
		deviceIds := make([]string, 0, len(devices))
		for _, d := range devices {
			deviceIds = append(deviceIds, d.Id)
		}
		// One query for the whole page's nodes, then grouped in memory.
		nodesByDevice := map[string][]Node{}
		if len(deviceIds) > 0 {
			nodeQuery := DB.Where("user_id = ? AND device_id IN ?", userId, deviceIds)
			if activeOnly {
				nodeQuery = nodeQuery.Where("state <> ? AND last_seen_at >= ?",
					NodeStateOffline, onlineCutoff)
			}
			var nodes []Node
			if err := nodeQuery.Order("last_seen_at desc").Find(&nodes).Error; err != nil {
				return nil, 0, err
			}
			for _, n := range nodes {
				nodesByDevice[n.DeviceId] = append(nodesByDevice[n.DeviceId], n)
			}
		}
		for i := range devices {
			device := devices[i]
			// A device with no nodes must serialize as [] not null, or the
			// console's node lookups (flatMap/map over row.nodes) crash on it.
			deviceNodes := nodesByDevice[device.Id]
			if deviceNodes == nil {
				deviceNodes = []Node{}
			}
			rows = append(rows, ConsoleRow{
				Device: &device,
				Nodes:  deviceNodes,
			})
		}
	}

	// Fill the rest of the page from the device-less node segment.
	if len(rows) < limit {
		orphanOffset := offset - int(deviceTotal)
		if orphanOffset < 0 {
			orphanOffset = 0
		}
		var orphans []Node
		if err := orphanNodeQuery().Order("last_seen_at desc, id desc").
			Offset(orphanOffset).Limit(limit - len(rows)).Find(&orphans).Error; err != nil {
			return nil, 0, err
		}
		for i := range orphans {
			rows = append(rows, ConsoleRow{Nodes: []Node{orphans[i]}})
		}
	}

	return rows, total, nil
}

// SetDeviceNickname sets (or clears, if empty) the user-defined nickname on a
// device. Returns ErrDeviceNotFound when the device doesn't belong to userId.
func SetDeviceNickname(userId int, deviceId, nickname string) error {
	res := DB.Model(&Device{}).
		Where("id = ? AND user_id = ?", deviceId, userId).
		Update("nickname", nickname)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// ErrDeviceNotRevoked is returned when trying to delete a device that is still
// active — only revoked devices may be purged.
var ErrDeviceNotRevoked = errors.New("only revoked devices can be deleted")

// DeleteRevokedDevice hard-deletes a revoked device and cascades to its nodes
// and their capabilities. Refuses to delete an active device (revoke first).
func DeleteRevokedDevice(userId int, deviceId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var d Device
		if err := tx.Where("id = ? AND user_id = ?", deviceId, userId).First(&d).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrDeviceNotFound
			}
			return err
		}
		if d.Status != DeviceStatusRevoked {
			return ErrDeviceNotRevoked
		}
		// Cascade: capabilities of the device's nodes, then nodes, then device.
		var nodeIds []string
		if err := tx.Model(&Node{}).Where("device_id = ?", deviceId).Pluck("id", &nodeIds).Error; err != nil {
			return err
		}
		if len(nodeIds) > 0 {
			if err := tx.Where("node_id IN ?", nodeIds).Delete(&NodeCapability{}).Error; err != nil {
				return err
			}
			if err := tx.Where("device_id = ?", deviceId).Delete(&Node{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", deviceId).Delete(&Device{}).Error
	})
}
