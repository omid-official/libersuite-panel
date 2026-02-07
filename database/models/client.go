package models

import (
	"time"

	"gorm.io/gorm"
)

// Client represents an SSH VPN client
type Client struct {
	gorm.Model
	Username       string    `gorm:"uniqueIndex;not null"`
	Password       string    `gorm:"not null"`
	TrafficLimit   int64     `gorm:"default:0"` // in bytes, 0 means unlimited
	TrafficUsed    int64     `gorm:"default:0"` // in bytes
	ExpiresAt      time.Time // expiration date
	Enabled        bool      `gorm:"default:true"`
	LastConnection time.Time
}

// IsExpired checks if the client's access has expired
func (c *Client) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// HasTrafficRemaining checks if the client has traffic quota remaining
func (c *Client) HasTrafficRemaining() bool {
	if c.TrafficLimit == 0 {
		return true
	}
	return c.TrafficUsed < c.TrafficLimit
}

// IsActive checks if the client can connect
func (c *Client) IsActive() bool {
	return c.Enabled && !c.IsExpired() && c.HasTrafficRemaining()
}

// RemainingTraffic returns the remaining traffic in bytes
func (c *Client) RemainingTraffic() int64 {
	if c.TrafficLimit == 0 {
		return -1 // unlimited
	}
	remaining := c.TrafficLimit - c.TrafficUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}
