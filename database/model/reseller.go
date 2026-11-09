package model

// ResellerAccess grants client management for exactly one existing inbound.
// Authentication secrets are never included in API responses or inbound exports.
type ResellerAccess struct {
	ID           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundID    int    `json:"inboundId" gorm:"uniqueIndex;not null"`
	Enabled      bool   `json:"enabled"`
	Path         string `json:"path" gorm:"uniqueIndex;not null"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Version      string `json:"-"`
}

type ResellerAudit struct {
	ID        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundID int    `json:"inboundId" gorm:"index"`
	Username  string `json:"username"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	IP        string `json:"ip"`
	CreatedAt int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
}
