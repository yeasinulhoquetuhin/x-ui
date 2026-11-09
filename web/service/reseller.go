package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

type ResellerService struct{}

type ResellerAccessForm struct {
	Enabled  bool   `json:"enabled" form:"enabled"`
	Path     string `json:"path" form:"path"`
	Username string `json:"username" form:"username"`
	Password string `json:"password" form:"password"`
}

var resellerSettingsMu sync.Mutex
var resellerPathPattern = regexp.MustCompile(`^[\p{L}\p{N}_.~-]+$`)
var resellerSubIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func ValidResellerPath(path string) bool {
	if !resellerPathPattern.MatchString(path) {
		return false
	}
	if path == "." || path == ".." {
		return false
	}
	switch strings.ToLower(path) {
	case "panel", "assets", "login", "logout", "ws", "xui", "api", "gettwofactorenable", "favicon", "robots":
		return false
	}
	return true
}

const resellerPasswordPrefix = "$xui-bcrypt-sha256$"

// Prehashing supports any password length without bcrypt's 72-byte truncation.
// Existing reseller bcrypt hashes remain valid after an in-place update.
func hashResellerPassword(password string) (string, error) {
	digest := sha256.Sum256([]byte(password))
	hash, err := crypto.HashPasswordAsBcrypt(base64.StdEncoding.EncodeToString(digest[:]))
	if err != nil {
		return "", err
	}
	return resellerPasswordPrefix + hash, nil
}

func CheckResellerPassword(hash, password string) bool {
	if strings.HasPrefix(hash, resellerPasswordPrefix) {
		digest := sha256.Sum256([]byte(password))
		return crypto.CheckPasswordHash(strings.TrimPrefix(hash, resellerPasswordPrefix), base64.StdEncoding.EncodeToString(digest[:]))
	}
	return crypto.CheckPasswordHash(hash, password)
}

func (s *ResellerService) Get(inboundID int) (*model.ResellerAccess, error) {
	var account model.ResellerAccess
	err := database.GetDB().Where("inbound_id = ?", inboundID).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.ResellerAccess{InboundID: inboundID}, nil
	}
	return &account, err
}

func (s *ResellerService) ByPath(path string) (*model.ResellerAccess, error) {
	if !ValidResellerPath(path) {
		return nil, gorm.ErrRecordNotFound
	}
	var account model.ResellerAccess
	err := database.GetDB().Where("path = ? AND enabled = ?", path, true).
		Where("inbound_id IN (SELECT id FROM inbounds)").First(&account).Error
	return &account, err
}

func resellerProtocolSupported(inbound *model.Inbound) bool {
	switch inbound.Protocol {
	case model.VLESS, model.VMESS, model.Trojan, model.Hysteria, model.Hysteria2:
		return true
	case model.Shadowsocks:
		var settings struct {
			Clients []model.Client `json:"clients"`
		}
		return json.Unmarshal([]byte(inbound.Settings), &settings) == nil && settings.Clients != nil
	}
	return false
}

func (s *ResellerService) Save(inboundID int, form ResellerAccessForm) (*model.ResellerAccess, error) {
	resellerSettingsMu.Lock()
	defer resellerSettingsMu.Unlock()
	var inbound model.Inbound
	if err := database.GetDB().First(&inbound, inboundID).Error; err != nil {
		return nil, err
	}
	account, err := s.Get(inboundID)
	if err != nil {
		return nil, err
	}
	if form.Enabled && !resellerProtocolSupported(&inbound) {
		return nil, errors.New("reseller access requires a multi-client inbound")
	}
	if form.Enabled {
		// A subscription ID shared with another inbound would expose that inbound
		// through its public subscription URL after assigning reseller access.
		var shared int64
		if err := database.GetDB().Raw(`SELECT COUNT(*) FROM inbounds AS own, json_each(own.settings, '$.clients') AS client
			WHERE own.id = ? AND COALESCE(json_extract(client.value, '$.subId'), '') <> ''
			AND EXISTS (SELECT 1 FROM inbounds AS other, json_each(other.settings, '$.clients') AS other_client
			WHERE other.id <> own.id AND json_extract(other_client.value, '$.subId') = json_extract(client.value, '$.subId'))`, inboundID).Scan(&shared).Error; err != nil {
			return nil, err
		}
		if shared > 0 {
			return nil, errors.New("this inbound shares subscription IDs with another inbound; use separate subscription IDs before enabling reseller access")
		}
		var inboundService InboundService
		clients, err := inboundService.GetClients(&inbound)
		if err != nil {
			return nil, err
		}
		var others []model.Inbound
		if err := database.GetDB().Where("id <> ?", inboundID).Find(&others).Error; err != nil {
			return nil, err
		}
		for _, other := range others {
			rows, err := inboundService.GetClients(&other)
			if err != nil {
				return nil, errors.New("unable to validate inbound client isolation")
			}
			for _, client := range clients {
				for _, candidate := range rows {
					if client.Email != "" && strings.EqualFold(client.Email, candidate.Email) || inbound.Protocol == other.Protocol && resellerAuthenticationID(inbound.Protocol, client) != "" && resellerAuthenticationID(inbound.Protocol, client) == resellerAuthenticationID(other.Protocol, candidate) {
						return nil, errors.New("this inbound shares client identities with another inbound; use separate client identities before enabling reseller access")
					}
				}
			}
		}
	}
	if !form.Enabled && account.ID == 0 && form.Path == "" {
		return account, nil
	}
	if !ValidResellerPath(form.Path) {
		return nil, errors.New("choose a non-empty URL path ID using letters, numbers, dots, underscores, hyphens or ~; this path must not be reserved")
	}
	if strings.TrimSpace(form.Username) == "" {
		return nil, errors.New("username is required")
	}
	var count int64
	if err := database.GetDB().Model(&model.ResellerAccess{}).Where("LOWER(path) = LOWER(?) AND inbound_id <> ?", form.Path, inboundID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("path ID is already in use")
	}
	changed := account.Enabled != form.Enabled || account.Path != form.Path || account.Username != form.Username
	if form.Password != "" {
		hash, err := hashResellerPassword(form.Password)
		if err != nil {
			return nil, err
		}
		account.PasswordHash = hash
		changed = true
	}
	if account.PasswordHash == "" {
		return nil, errors.New("a password is required for new reseller access")
	}
	account.Enabled, account.Path, account.Username = form.Enabled, form.Path, form.Username
	if changed || account.Version == "" {
		account.Version = uuid.NewString()
	}
	if err := database.GetDB().Save(account).Error; err != nil {
		return nil, err
	}
	return account, nil
}

func (s *ResellerService) Inbound(inboundID int) (*model.Inbound, error) {
	var inbound model.Inbound
	if err := database.GetDB().Preload("ClientStats").First(&inbound, inboundID).Error; err != nil {
		return nil, err
	}
	if strings.HasPrefix(inbound.Listen, "@") {
		// Publish only the public connection endpoint for a Unix-socket fallback,
		// not the parent inbound or any of its clients/settings.
		var parent model.Inbound
		err := database.GetDB().Where("enable = ?", true).
			Where("EXISTS (SELECT 1 FROM json_each(settings, '$.fallbacks') WHERE json_extract(value, '$.dest') = ?)", inbound.Listen).First(&parent).Error
		if err == nil {
			inbound.Listen = parent.Listen
			inbound.Port = parent.Port
			inbound.StreamSettings = parent.StreamSettings
		}
	}
	// Keep only fields needed to manage clients and generate their connection links.
	var settings map[string]json.RawMessage
	if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
		return nil, err
	}
	public := make(map[string]json.RawMessage)
	for _, key := range []string{"clients", "encryption", "method", "password", "network", "version"} {
		if value, ok := settings[key]; ok {
			public[key] = value
		}
	}
	data, _ := json.Marshal(public)
	inbound.Settings = string(data)
	var stream map[string]any
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil && inbound.StreamSettings != "" {
		return nil, err
	}
	redactResellerServerSecrets(stream)
	data, _ = json.Marshal(stream)
	inbound.StreamSettings = string(data)
	inbound.Sniffing = "{}"
	return &inbound, nil
}

func redactResellerServerSecrets(value any) {
	switch data := value.(type) {
	case map[string]any:
		for key, child := range data {
			switch strings.ToLower(key) {
			case "privatekey", "mldsa65seed", "certificates", "keyfile", "certificatefile", "masterkeylog", "echserverkeys", "sockopt", "seed":
				delete(data, key)
			default:
				redactResellerServerSecrets(child)
			}
		}
	case []any:
		for _, child := range data {
			redactResellerServerSecrets(child)
		}
	}
}

func ResellerClientID(protocol model.Protocol, client model.Client) string {
	switch protocol {
	case model.Trojan:
		return client.Password
	case model.Shadowsocks:
		return client.Email
	case model.Hysteria, model.Hysteria2:
		return client.Auth
	default:
		return client.ID
	}
}

func resellerAuthenticationID(protocol model.Protocol, client model.Client) string {
	identity := ResellerClientID(protocol, client)
	if protocol == model.VLESS || protocol == model.VMESS {
		if parsed, err := uuid.Parse(identity); err == nil {
			return parsed.String()
		}
	}
	return identity
}

// ValidateClients prevents a reseller from joining another account's subscription
// or reusing its authentication identity through the shared underlying core.
func (s *ResellerService) ValidateClients(inbound *model.Inbound, clients []model.Client, replacing string) error {
	if len(clients) == 0 || replacing != "" && len(clients) != 1 {
		return errors.New("provide the client records to manage")
	}
	var inbounds []model.Inbound
	if err := database.GetDB().Find(&inbounds).Error; err != nil {
		return err
	}
	var inboundService InboundService
	for i, client := range clients {
		if strings.TrimSpace(client.Email) == "" || len(client.Email) > 128 || strings.ContainsAny(client.Email, "\r\n\x00") || strings.Contains(client.Email, ">>>") {
			return errors.New("each client requires a valid email/name")
		}
		if client.SubID != "" && !resellerSubIDPattern.MatchString(client.SubID) {
			return errors.New("subscription ID must use letters, digits, underscores or hyphens")
		}
		if ResellerClientID(inbound.Protocol, client) == "" {
			return errors.New("client authentication ID is required")
		}
		for _, candidate := range inbounds {
			others, err := inboundService.GetClients(&candidate)
			if err != nil {
				return errors.New("unable to validate client identities")
			}
			for _, other := range others {
				if candidate.Id == inbound.Id && replacing != "" && ResellerClientID(candidate.Protocol, other) == replacing {
					continue
				}
				if strings.EqualFold(client.Email, other.Email) || candidate.Protocol == inbound.Protocol && resellerAuthenticationID(inbound.Protocol, client) == resellerAuthenticationID(candidate.Protocol, other) || client.SubID != "" && client.SubID == other.SubID && candidate.Id != inbound.Id {
					return errors.New("client identity or subscription ID is already in use")
				}
			}
		}
		for j := 0; j < i; j++ {
			if strings.EqualFold(client.Email, clients[j].Email) || resellerAuthenticationID(inbound.Protocol, client) == resellerAuthenticationID(inbound.Protocol, clients[j]) {
				return errors.New("duplicate client identity in request")
			}
		}
	}
	return nil
}

func (s *ResellerService) Online(inboundID int) ([]string, map[string]int64, error) {
	var rows []xray.ClientTraffic
	if err := database.GetDB().Where("inbound_id = ?", inboundID).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	var inboundService InboundService
	online := make(map[string]bool)
	if p != nil {
		for _, email := range inboundService.GetOnlineClients() {
			online[email] = true
		}
	}
	emails := make([]string, 0)
	last := make(map[string]int64)
	for _, row := range rows {
		if online[row.Email] {
			emails = append(emails, row.Email)
		}
		last[row.Email] = row.LastOnline
	}
	return emails, last, nil
}

func (s *ResellerService) Audit(account *model.ResellerAccess, action, target, ip string) error {
	return database.GetDB().Create(&model.ResellerAudit{InboundID: account.InboundID, Username: account.Username, Action: action, Target: target, IP: ip}).Error
}

func (s *ResellerService) AuditList(inboundID int) ([]model.ResellerAudit, error) {
	var rows []model.ResellerAudit
	err := database.GetDB().Where("inbound_id = ?", inboundID).Order("id DESC").Limit(100).Find(&rows).Error
	return rows, err
}

func (s *ResellerService) OwnsEmail(inboundID int, email string) bool {
	var count int64
	return database.GetDB().Model(&xray.ClientTraffic{}).Where("inbound_id = ? AND email = ?", inboundID, email).Count(&count).Error == nil && count == 1
}

func (s *ResellerService) ClientInput(inboundID int, settings string, replacing string) (*model.Inbound, error) {
	var inbound model.Inbound
	if err := database.GetDB().First(&inbound, inboundID).Error; err != nil {
		return nil, err
	}
	var request struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(settings), &request); err != nil {
		return nil, fmt.Errorf("invalid client settings: %w", err)
	}
	if err := s.ValidateClients(&inbound, request.Clients, replacing); err != nil {
		return nil, err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	return &model.Inbound{Id: inboundID, Settings: string(data)}, nil
}
