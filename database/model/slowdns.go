package model

// SlowDNSConfig is separate from resettable panel settings. Uninstalling the
// tunnel retains its keypair; only an explicit regenerate action replaces it.
type SlowDNSConfig struct {
	ID         int    `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Installed  bool   `json:"installed"`
	Enabled    bool   `json:"enabled"`
	HostDomain string `json:"hostDomain,omitempty"`
	NSDomain   string `json:"nsDomain,omitempty"`
	Listen     string `json:"listen,omitempty"`
	MTU        int    `json:"mtu"`
	PrivateKey string `json:"-"`
	PublicKey  string `json:"publicKey,omitempty"`
}
