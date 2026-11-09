package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
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
	Enabled         bool   `json:"enabled" form:"enabled"`
	Path            string `json:"path" form:"path"`
	Username        string `json:"username" form:"username"`
	Password        string `json:"password" form:"password"`
	CanEditInbounds bool   `json:"canEditInbounds" form:"canEditInbounds"`
	InboundIDs      []int  `json:"inboundIds" form:"inboundIds"`
}

// Assignment/permission updates cannot race an authenticated reseller write.
var ResellerAccessMu sync.RWMutex
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

func (s *ResellerService) Get(adminID, id int) (*model.ResellerAccount, error) {
	var account model.ResellerAccount
	if err := database.GetDB().Where("admin_id = ?", adminID).First(&account, id).Error; err != nil {
		return nil, err
	}
	ids, err := s.AssignedIDs(&account)
	account.InboundIDs = ids
	return &account, err
}

func (s *ResellerService) List(adminID int) ([]model.ResellerAccount, error) {
	accounts := make([]model.ResellerAccount, 0)
	if err := database.GetDB().Where("admin_id = ?", adminID).Order("id").Find(&accounts).Error; err != nil {
		return nil, err
	}
	for i := range accounts {
		ids, err := s.AssignedIDs(&accounts[i])
		if err != nil {
			return nil, err
		}
		accounts[i].InboundIDs = ids
	}
	return accounts, nil
}

func (s *ResellerService) AssignedIDs(account *model.ResellerAccount) ([]int, error) {
	ids := make([]int, 0)
	err := database.GetDB().Model(&model.ResellerInbound{}).
		Where("reseller_id = ?", account.ID).
		Where("inbound_id IN (SELECT id FROM inbounds WHERE user_id = ?)", account.AdminID).
		Order("inbound_id").Pluck("inbound_id", &ids).Error
	return ids, err
}

func (s *ResellerService) HasInbound(account *model.ResellerAccount, id int) bool {
	var count int64
	return id > 0 && database.GetDB().Model(&model.ResellerInbound{}).
		Where("reseller_id = ? AND inbound_id = ?", account.ID, id).
		Where("inbound_id IN (SELECT id FROM inbounds WHERE user_id = ?)", account.AdminID).
		Count(&count).Error == nil && count == 1
}

func (s *ResellerService) ByPath(path string) (*model.ResellerAccount, error) {
	if !ValidResellerPath(path) {
		return nil, gorm.ErrRecordNotFound
	}
	var account model.ResellerAccount
	err := database.GetDB().Where("path = ? AND enabled = ?", path, true).First(&account).Error
	return &account, err
}

func (s *ResellerService) Save(adminID, id int, form ResellerAccessForm) (*model.ResellerAccount, error) {
	ResellerAccessMu.Lock()
	defer ResellerAccessMu.Unlock()
	account := &model.ResellerAccount{AdminID: adminID}
	if id > 0 {
		var err error
		account, err = s.Get(adminID, id)
		if err != nil {
			return nil, err
		}
	}
	if !ValidResellerPath(form.Path) {
		return nil, errors.New("choose a non-empty URL path ID using letters, numbers, dots, underscores, hyphens or ~; this path must not be reserved")
	}
	if strings.TrimSpace(form.Username) == "" {
		return nil, errors.New("username is required")
	}
	var count int64
	if err := database.GetDB().Model(&model.ResellerAccount{}).Where("LOWER(path) = LOWER(?) AND id <> ?", form.Path, id).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("path ID is already in use")
	}
	ids := append([]int{}, form.InboundIDs...)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > 0 {
		if ids[0] <= 0 {
			return nil, errors.New("invalid inbound assignment")
		}
		if err := database.GetDB().Model(&model.Inbound{}).Where("user_id = ? AND id IN ?", adminID, ids).Count(&count).Error; err != nil {
			return nil, err
		}
		if count != int64(len(ids)) {
			return nil, errors.New("one or more selected inbounds do not belong to this administrator")
		}
		if form.Enabled {
			if err := s.validateAssignmentIsolation(ids); err != nil {
				return nil, err
			}
		}
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
	account.CanEditInbounds = form.CanEditInbounds
	if changed || account.Version == "" {
		account.Version = uuid.NewString()
	}
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(account).Error; err != nil {
			return err
		}
		if err := tx.Where("reseller_id = ?", account.ID).Delete(&model.ResellerInbound{}).Error; err != nil {
			return err
		}
		for _, inboundID := range ids {
			if err := tx.Create(&model.ResellerInbound{ResellerID: account.ID, InboundID: inboundID}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	account.InboundIDs = ids
	return account, nil
}

func (s *ResellerService) Delete(adminID, id int) error {
	ResellerAccessMu.Lock()
	defer ResellerAccessMu.Unlock()
	if _, err := s.Get(adminID, id); err != nil {
		return err
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("reseller_id = ?", id).Delete(&model.ResellerInbound{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.ResellerAccount{}, id).Error
	})
}

func (s *ResellerService) validateAssignmentIsolation(ids []int) error {
	var inbounds []model.Inbound
	if err := database.GetDB().Find(&inbounds).Error; err != nil {
		return err
	}
	clients := make(map[int][]model.Client)
	for _, inbound := range inbounds {
		var settings struct {
			Clients []model.Client `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return fmt.Errorf("unable to read clients for inbound %d", inbound.Id)
		}
		clients[inbound.Id] = settings.Clients
	}
	for _, id := range ids {
		for otherID, rows := range clients {
			if id == otherID {
				continue
			}
			for _, own := range clients[id] {
				for _, other := range rows {
					if own.Email != "" && strings.EqualFold(own.Email, other.Email) {
						return fmt.Errorf("client email/name %q is also used by inbound %d; use a separate email/name", own.Email, otherID)
					}
					if own.SubID != "" && own.SubID == other.SubID && !slices.Contains(ids, otherID) {
						return fmt.Errorf("inbound %d shares a subscription with unassigned inbound %d; assign both or use separate subscription IDs", id, otherID)
					}
				}
			}
		}
	}
	return nil
}

func (s *ResellerService) Inbounds(account *model.ResellerAccount) ([]*model.Inbound, error) {
	ids, err := s.AssignedIDs(account)
	if err != nil {
		return nil, err
	}
	rows := make([]*model.Inbound, 0, len(ids))
	for _, id := range ids {
		inbound, err := s.Inbound(id, account.CanEditInbounds)
		if err != nil {
			return nil, err
		}
		rows = append(rows, inbound)
	}
	return rows, nil
}

func (s *ResellerService) Inbound(inboundID int, editable ...bool) (*model.Inbound, error) {
	var inbound model.Inbound
	if err := database.GetDB().Preload("ClientStats").First(&inbound, inboundID).Error; err != nil {
		return nil, err
	}
	if len(editable) > 0 && editable[0] {
		return &inbound, nil
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

// ValidateClients keeps global accounting/subscriptions isolated and prevents
// ambiguous authentication identities inside the assigned inbound.
func (s *ResellerService) ValidateClients(inbound *model.Inbound, clients []model.Client, replacing string) error {
	return s.validateClients(inbound, clients, replacing, nil)
}

func (s *ResellerService) validateClients(inbound *model.Inbound, clients []model.Client, replacing string, assigned []int) error {
	if len(clients) == 0 || replacing != "" && len(clients) != 1 {
		return errors.New("provide the client records to manage")
	}
	var inbounds []model.Inbound
	if err := database.GetDB().Find(&inbounds).Error; err != nil {
		return err
	}
	var inboundService InboundService
	var existing []model.Client
	if replacing != "" {
		var err error
		existing, err = inboundService.GetClients(inbound)
		if err != nil {
			return err
		}
	}
	for i, client := range clients {
		keepSharedSubscription := false
		for _, old := range existing {
			if ResellerClientID(inbound.Protocol, old) == replacing && old.SubID == client.SubID {
				keepSharedSubscription = true
			}
		}
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
				if strings.EqualFold(client.Email, other.Email) || candidate.Id == inbound.Id && resellerAuthenticationID(inbound.Protocol, client) == resellerAuthenticationID(candidate.Protocol, other) || client.SubID != "" && client.SubID == other.SubID && candidate.Id != inbound.Id && !(keepSharedSubscription && slices.Contains(assigned, candidate.Id)) {
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

func (s *ResellerService) Online(account *model.ResellerAccount) ([]string, map[string]int64, error) {
	ids, err := s.AssignedIDs(account)
	if err != nil {
		return nil, nil, err
	}
	var rows []xray.ClientTraffic
	if err := database.GetDB().Where("inbound_id IN ?", ids).Find(&rows).Error; err != nil {
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

func (s *ResellerService) Audit(account *model.ResellerAccount, inboundID int, action, target, ip string) error {
	return database.GetDB().Create(&model.ResellerAudit{ResellerID: account.ID, InboundID: inboundID, Username: account.Username, Action: action, Target: target, IP: ip}).Error
}

func (s *ResellerService) AuditList(resellerID int) ([]model.ResellerAudit, error) {
	var rows []model.ResellerAudit
	err := database.GetDB().Where("reseller_id = ?", resellerID).Order("id DESC").Limit(100).Find(&rows).Error
	return rows, err
}

func (s *ResellerService) EmailInbound(account *model.ResellerAccount, email string) (int, error) {
	var row xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", email).First(&row).Error; err != nil {
		return 0, err
	}
	if !s.HasInbound(account, row.InboundId) {
		return 0, errors.New("client is not assigned to your account")
	}
	return row.InboundId, nil
}

func (s *ResellerService) ClientInput(inboundID int, settings string, replacing string, accounts ...*model.ResellerAccount) (*model.Inbound, error) {
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
	var assigned []int
	if len(accounts) > 0 {
		var err error
		assigned, err = s.AssignedIDs(accounts[0])
		if err != nil {
			return nil, err
		}
	}
	if err := s.validateClients(&inbound, request.Clients, replacing, assigned); err != nil {
		return nil, err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	return &model.Inbound{Id: inboundID, Settings: string(data)}, nil
}

// InboundInput accepts configuration edits only; clients and traffic counters
// come from the latest DB record so a stale editor cannot undo shared changes.
func (s *ResellerService) InboundInput(account *model.ResellerAccount, id int, input *model.Inbound) (*model.Inbound, error) {
	if !account.CanEditInbounds || !s.HasInbound(account, id) {
		return nil, errors.New("inbound editing is not permitted")
	}
	var current model.Inbound
	if err := database.GetDB().First(&current, id).Error; err != nil {
		return nil, err
	}
	if input.Protocol != current.Protocol {
		return nil, errors.New("the inbound protocol cannot be changed")
	}
	if input.Port < 1 || input.Port > 65535 || input.Total < 0 {
		return nil, errors.New("invalid inbound port or quota")
	}
	if input.TrafficReset != "" && input.TrafficReset != "never" && input.TrafficReset != "hourly" && input.TrafficReset != "daily" && input.TrafficReset != "weekly" && input.TrafficReset != "monthly" {
		return nil, errors.New("invalid inbound traffic reset")
	}
	var settings, original map[string]json.RawMessage
	if json.Unmarshal([]byte(input.Settings), &settings) != nil || settings == nil || json.Unmarshal([]byte(current.Settings), &original) != nil {
		return nil, errors.New("invalid inbound settings")
	}
	delete(settings, "clients")
	if clients, ok := original["clients"]; ok {
		settings["clients"] = clients
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	if !json.Valid([]byte(input.StreamSettings)) || !json.Valid([]byte(input.Sniffing)) {
		return nil, errors.New("invalid stream or sniffing settings")
	}
	input.Id, input.UserId = current.Id, current.UserId
	input.Up, input.Down, input.AllTime = current.Up, current.Down, current.AllTime
	input.LastTrafficResetTime = current.LastTrafficResetTime
	input.Settings = string(data)
	return input, nil
}

func (s *ResellerService) UpdateInbound(account *model.ResellerAccount, id int, input *model.Inbound) (bool, error) {
	clientTrafficMu.Lock()
	defer clientTrafficMu.Unlock()
	data, err := s.InboundInput(account, id, input)
	if err != nil {
		return false, err
	}
	var inboundService InboundService
	_, restart, err := inboundService.UpdateInbound(data)
	return restart, err
}

func (s *ResellerService) ToggleInbound(account *model.ResellerAccount, id int, enabled bool) (bool, error) {
	clientTrafficMu.Lock()
	defer clientTrafficMu.Unlock()
	if !account.CanEditInbounds || !s.HasInbound(account, id) {
		return false, errors.New("inbound editing is not permitted")
	}
	var inboundService InboundService
	data, err := inboundService.GetInbound(id)
	if err != nil {
		return false, err
	}
	data.Enable = enabled
	_, restart, err := inboundService.UpdateInbound(data)
	return restart, err
}
