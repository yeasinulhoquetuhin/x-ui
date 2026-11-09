package model

// ResellerAccess is the v2 per-inbound record, retained for one-time migration.
type ResellerAccess struct {
	ID           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundID    int    `json:"inboundId" gorm:"uniqueIndex;not null"`
	Enabled      bool   `json:"enabled"`
	Path         string `json:"path" gorm:"uniqueIndex;not null"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Version      string `json:"-"`
}

// ResellerAccount is independent of its many-to-many inbound assignments.
type ResellerAccount struct {
	ID              int    `json:"id" gorm:"primaryKey;autoIncrement"`
	AdminID         int    `json:"-" gorm:"index;not null"`
	Enabled         bool   `json:"enabled"`
	CanEditInbounds bool   `json:"canEditInbounds" gorm:"default:false"`
	Path            string `json:"path" gorm:"uniqueIndex;not null"`
	Username        string `json:"username"`
	PasswordHash    string `json:"-"`
	Version         string `json:"-"`
	InboundIDs      []int  `json:"inboundIds" gorm:"-"`
	CreatedAt       int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

type ResellerInbound struct {
	ResellerID int `json:"resellerId" gorm:"primaryKey;autoIncrement:false"`
	InboundID  int `json:"inboundId" gorm:"primaryKey;autoIncrement:false;index"`
}

type ResellerAudit struct {
	ID         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ResellerID int    `json:"resellerId" gorm:"index"`
	InboundID  int    `json:"inboundId" gorm:"index"`
	Username   string `json:"username"`
	Action     string `json:"action"`
	Target     string `json:"target"`
	IP         string `json:"ip"`
	CreatedAt  int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
}
