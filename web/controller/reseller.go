package controller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
)

type ResellerController struct {
	service  service.ResellerService
	inbound  service.InboundService
	core     service.XrayService
	settings service.SettingService
	clients  InboundController
	limiter  resellerLoginLimiter
}

type resellerLoginAttempt struct {
	Count int
	Until time.Time
}
type resellerLoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]resellerLoginAttempt
}

var resellerClientWriteMu sync.Mutex

func (l *resellerLoginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.attempts == nil {
		l.attempts = make(map[string]resellerLoginAttempt)
	}
	for k, value := range l.attempts {
		if !now.Before(value.Until) {
			delete(l.attempts, k)
		}
	}
	value := l.attempts[key]
	if value.Count >= 10 {
		return false
	}
	if len(l.attempts) >= 4096 && value.Count == 0 {
		return false
	}
	if value.Count == 0 {
		value.Until = now.Add(5 * time.Minute)
	}
	value.Count++
	l.attempts[key] = value
	return true
}

func (l *resellerLoginLimiter) clear(key string) { l.mu.Lock(); delete(l.attempts, key); l.mu.Unlock() }

func NewResellerController(g *gin.RouterGroup, secret []byte) *ResellerController {
	a := &ResellerController{}
	key := sha256.Sum256(append(append([]byte{}, secret...), []byte("tdz-reseller-session")...))
	store := cookie.NewStore(key[:])
	portal := g.Group("/:resellerPath")
	portal.Use(sessions.Sessions("tdz-reseller", store), a.resolve)
	portal.GET("/", a.index)
	portal.GET("/assets/*asset", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, c.GetString("asset_path")+"assets/"+strings.TrimPrefix(c.Param("asset"), "/"))
	})
	portal.POST("/login", a.login)
	portal.POST("/getTwoFactorEnable", func(c *gin.Context) { jsonObj(c, false, nil) })
	portal.GET("/logout", a.logout)
	portal.POST("/logout", a.logout)
	secured := portal.Group("")
	secured.Use(a.authenticate)
	secured.GET("/panel/", a.dashboard)
	secured.GET("/panel/inbounds", a.dashboard)
	secured.POST("/panel/setting/defaultSettings", a.defaults)
	secured.GET("/ws", a.websocket)
	keys := secured.Group("/panel/api/server")
	keys.Use(a.requireInboundEditing)
	server := &ServerController{}
	keys.GET("/getNewUUID", server.getNewUUID)
	keys.GET("/getNewX25519Cert", server.getNewX25519Cert)
	keys.GET("/getNewmldsa65", server.getNewmldsa65)
	keys.GET("/getNewmlkem768", server.getNewmlkem768)
	keys.GET("/getNewVlessEnc", server.getNewVlessEnc)
	keys.POST("/getNewEchCert", server.getNewEchCert)
	api := secured.Group("/panel/api/inbounds")
	api.GET("/list", a.list)
	api.POST("/onlines", a.onlines)
	api.POST("/lastOnline", a.lastOnline)
	api.POST("/addClient", a.addClient)
	api.POST("/updateClient/:clientId", a.updateClient)
	api.POST("/:id/delClient/:clientId", a.deleteClient)
	api.POST("/:id/resetClientTraffic/:email", a.resetClient)
	api.POST("/resetAllClientTraffics/:id", a.resetClients)
	api.POST("/delDepletedClients/:id", a.deleteDepleted)
	api.POST("/clientIps/:email", a.clientIPs)
	api.POST("/clearClientIps/:email", a.clearIPs)
	api.POST("/update/:id", a.editInbound)
	api.POST("/toggle/:id", a.toggleInbound)
	return a
}

func resellerPeer(c *gin.Context) string {
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

func (a *ResellerController) resolve(c *gin.Context) {
	account, err := a.service.ByPath(c.Param("resellerPath"))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	root := c.GetString("base_path")
	c.Set("asset_path", root)
	c.Set("base_path", root+url.PathEscape(account.Path)+"/")
	c.Set("is_reseller", true)
	c.Set("reseller_account", account)
	c.Header("Cache-Control", "no-store")
	c.Header("X-Frame-Options", "DENY")
	if c.Request.Method != http.MethodGet {
		origin := c.GetHeader("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host != c.Request.Host || parsed.Scheme != "http" && parsed.Scheme != "https" {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
		}
		if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
	}
	c.Next()
}

func (a *ResellerController) account(c *gin.Context) *model.ResellerAccount {
	return c.MustGet("reseller_account").(*model.ResellerAccount)
}

func (a *ResellerController) authenticate(c *gin.Context) {
	service.ResellerAccessMu.RLock()
	defer service.ResellerAccessMu.RUnlock()
	account := session.GetReseller(c)
	if account == nil {
		if isAjax(c) || strings.Contains(c.Request.URL.Path, "/api/") {
			pureJsonMsg(c, http.StatusUnauthorized, false, "Please log in again")
		} else {
			c.Redirect(http.StatusTemporaryRedirect, c.GetString("base_path"))
		}
		c.Abort()
		return
	}
	c.Set("reseller_account", account)
	c.Next()
}

func (a *ResellerController) index(c *gin.Context) {
	if session.GetReseller(c) != nil {
		c.Redirect(http.StatusTemporaryRedirect, c.GetString("base_path")+"panel/")
		return
	}
	html(c, "login.html", "pages.login.title", gin.H{"reseller_name": a.account(c).Username})
}

func (a *ResellerController) login(c *gin.Context) {
	account := a.account(c)
	key := resellerPeer(c) + ":" + strconv.Itoa(account.ID)
	if !a.limiter.allow(key) {
		pureJsonMsg(c, http.StatusTooManyRequests, false, "Too many login attempts. Try again in five minutes.")
		return
	}
	var form LoginForm
	if c.ShouldBind(&form) != nil || form.Username != account.Username || !service.CheckResellerPassword(account.PasswordHash, form.Password) {
		pureJsonMsg(c, http.StatusOK, false, "Invalid username or password")
		return
	}
	a.limiter.clear(key)
	s := sessions.Default(c)
	s.Clear()
	minutes, _ := a.settings.GetSessionMaxAge()
	if minutes <= 0 {
		minutes = 60
	}
	s.Options(sessions.Options{Path: c.GetString("base_path"), MaxAge: minutes * 60, HttpOnly: true, Secure: c.Request.TLS != nil, SameSite: http.SameSiteLaxMode})
	session.SetReseller(c, account)
	if err := s.Save(); err != nil {
		jsonObj(c, nil, err)
		return
	}
	a.audit(c, 0, "login", "")
	jsonMsg(c, "Logged in", nil)
}

func (a *ResellerController) logout(c *gin.Context) {
	s := sessions.Default(c)
	s.Clear()
	s.Options(sessions.Options{Path: c.GetString("base_path"), MaxAge: -1, HttpOnly: true, Secure: c.Request.TLS != nil, SameSite: http.SameSiteLaxMode})
	_ = s.Save()
	c.Redirect(http.StatusSeeOther, c.GetString("base_path"))
}

func (a *ResellerController) dashboard(c *gin.Context) {
	html(c, "inbounds.html", "pages.inbounds.title", gin.H{"reseller_name": a.account(c).Username})
}

func (a *ResellerController) defaults(c *gin.Context) {
	value, err := a.settings.GetDefaultSettings(c.Request.Host)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	defaults := value.(map[string]any)
	delete(defaults, "defaultCert")
	delete(defaults, "defaultKey")
	defaults["canEditInbounds"] = a.account(c).CanEditInbounds
	jsonObj(c, defaults, nil)
}

func (a *ResellerController) list(c *gin.Context) {
	inbounds, err := a.service.Inbounds(a.account(c))
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	jsonObj(c, inbounds, nil)
}

func (a *ResellerController) onlines(c *gin.Context) {
	online, _, err := a.service.Online(a.account(c))
	jsonObj(c, online, err)
}
func (a *ResellerController) lastOnline(c *gin.Context) {
	_, last, err := a.service.Online(a.account(c))
	jsonObj(c, last, err)
}

func (a *ResellerController) scopeID(c *gin.Context, id int) bool {
	if !a.service.HasInbound(a.account(c), id) {
		pureJsonMsg(c, http.StatusForbidden, false, "This inbound is not assigned to your account")
		return false
	}
	c.Set("reseller_inbound_id", id)
	return true
}

func (a *ResellerController) scopePath(c *gin.Context) bool {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return false
	}
	return a.scopeID(c, id)
}
func (a *ResellerController) scopeEmail(c *gin.Context) bool {
	id, err := a.service.EmailInbound(a.account(c), c.Param("email"))
	if err != nil || c.GetInt("reseller_inbound_id") != 0 && c.GetInt("reseller_inbound_id") != id {
		pureJsonMsg(c, http.StatusForbidden, false, "Client is not assigned to your account")
		return false
	}
	c.Set("reseller_inbound_id", id)
	return true
}

func (a *ResellerController) mutate(c *gin.Context, replacing string) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	var form struct {
		ID       int    `json:"id" form:"id"`
		Settings string `json:"settings" form:"settings"`
	}
	if err := c.ShouldBind(&form); err != nil {
		jsonObj(c, nil, err)
		return
	}
	if !a.scopeID(c, form.ID) {
		return
	}
	if replacing != "" && !a.ownsClient(c, form.ID, replacing) {
		return
	}
	data, err := a.service.ClientInput(form.ID, form.Settings, replacing, a.account(c))
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	var restart bool
	if replacing == "" {
		restart, err = a.inbound.AddInboundClient(data)
	} else {
		restart, err = a.inbound.UpdateInboundClient(data, replacing)
	}
	if err == nil {
		if restart {
			a.core.SetToNeedRestart()
		}
		if publishErr := service.WriteClientIPLimits(); publishErr != nil {
			logger.Warningf("Publish reseller IP policy: %v", publishErr)
		}
		action := "client.create"
		if replacing != "" {
			action = "client.update"
		}
		var settings struct {
			Clients []model.Client `json:"clients"`
		}
		_ = json.Unmarshal([]byte(data.Settings), &settings)
		for _, client := range settings.Clients {
			a.audit(c, form.ID, action, client.Email)
		}
	}
	jsonMsg(c, "Client saved", err)
}

func (a *ResellerController) addClient(c *gin.Context)    { a.mutate(c, "") }
func (a *ResellerController) updateClient(c *gin.Context) { a.mutate(c, c.Param("clientId")) }

func (a *ResellerController) ownsClient(c *gin.Context, inboundID int, id string) bool {
	inbound, err := a.inbound.GetInbound(inboundID)
	if err == nil {
		clients, err := a.inbound.GetClients(inbound)
		if err == nil {
			for _, client := range clients {
				if service.ResellerClientID(inbound.Protocol, client) == id {
					return true
				}
			}
		}
	}
	pureJsonMsg(c, http.StatusForbidden, false, "Client is not assigned to your account")
	return false
}

func (a *ResellerController) deleteClient(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.scopePath(c) || !a.ownsClient(c, c.GetInt("reseller_inbound_id"), c.Param("clientId")) {
		return
	}
	restart, err := a.inbound.DelInboundClient(c.GetInt("reseller_inbound_id"), c.Param("clientId"))
	if err == nil {
		if restart {
			a.core.SetToNeedRestart()
		}
		a.audit(c, c.GetInt("reseller_inbound_id"), "client.delete", "client")
	}
	jsonMsg(c, "Client deleted", err)
}
func (a *ResellerController) resetClient(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.scopePath(c) || !a.scopeEmail(c) {
		return
	}
	restart, err := a.inbound.ResetClientTraffic(c.GetInt("reseller_inbound_id"), c.Param("email"))
	if err == nil {
		if restart {
			a.core.SetToNeedRestart()
		}
		a.audit(c, c.GetInt("reseller_inbound_id"), "client.reset", c.Param("email"))
	}
	jsonMsg(c, "Client traffic reset", err)
}
func (a *ResellerController) resetClients(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.scopePath(c) {
		return
	}
	err := a.inbound.ResetAllClientTraffics(c.GetInt("reseller_inbound_id"))
	if err == nil {
		a.core.SetToNeedRestart()
		a.audit(c, c.GetInt("reseller_inbound_id"), "clients.reset", "all")
	}
	jsonMsg(c, "Client traffic reset", err)
}
func (a *ResellerController) deleteDepleted(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.scopePath(c) {
		return
	}
	err := a.inbound.DelDepletedClients(c.GetInt("reseller_inbound_id"))
	if err == nil {
		a.audit(c, c.GetInt("reseller_inbound_id"), "clients.deleteDepleted", "depleted")
	}
	jsonMsg(c, "Depleted clients deleted", err)
}
func (a *ResellerController) clientIPs(c *gin.Context) {
	if a.scopeEmail(c) {
		a.clients.getClientIps(c)
	}
}
func (a *ResellerController) clearIPs(c *gin.Context) {
	if a.scopeEmail(c) {
		a.clients.clearClientIps(c)
	}
}

func (a *ResellerController) audit(c *gin.Context, inboundID int, action, target string) {
	if err := a.service.Audit(a.account(c), inboundID, action, target, resellerPeer(c)); err != nil {
		logger.Warningf("Reseller audit: %v", err)
	}
}

func (a *ResellerController) requireInboundEditing(c *gin.Context) {
	if !a.account(c).CanEditInbounds {
		pureJsonMsg(c, http.StatusForbidden, false, "Inbound editing is disabled for your account")
		c.Abort()
		return
	}
	c.Next()
}

func (a *ResellerController) editInbound(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.account(c).CanEditInbounds || !a.scopePath(c) {
		if !c.Writer.Written() {
			pureJsonMsg(c, http.StatusForbidden, false, "Inbound editing is disabled for your account")
		}
		return
	}
	var input model.Inbound
	if err := c.ShouldBind(&input); err != nil {
		jsonObj(c, nil, err)
		return
	}
	id := c.GetInt("reseller_inbound_id")
	restart, err := a.service.UpdateInbound(a.account(c), id, &input)
	if err == nil {
		if restart {
			a.core.SetToNeedRestart()
		}
		if publishErr := service.WriteClientIPLimits(); publishErr != nil {
			logger.Warningf("Publish reseller IP policy: %v", publishErr)
		}
		a.audit(c, id, "inbound.update", strconv.Itoa(id))
	}
	jsonMsg(c, "Inbound updated", err)
}

func (a *ResellerController) toggleInbound(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.account(c).CanEditInbounds || !a.scopePath(c) {
		if !c.Writer.Written() {
			pureJsonMsg(c, http.StatusForbidden, false, "Inbound editing is disabled for your account")
		}
		return
	}
	var form struct {
		Enabled *bool `json:"enabled" form:"enabled"`
	}
	if err := c.ShouldBind(&form); err != nil || form.Enabled == nil {
		pureJsonMsg(c, 400, false, "Enabled must be true or false")
		return
	}
	id := c.GetInt("reseller_inbound_id")
	restart, err := a.service.ToggleInbound(a.account(c), id, *form.Enabled)
	if err == nil {
		if restart {
			a.core.SetToNeedRestart()
		}
		if publishErr := service.WriteClientIPLimits(); publishErr != nil {
			logger.Warningf("Publish reseller IP policy: %v", publishErr)
		}
		a.audit(c, id, "inbound.toggle", strconv.FormatBool(*form.Enabled))
	}
	jsonMsg(c, "Inbound updated", err)
}

// A dedicated stream never subscribes to the global admin broadcast hub.
func (a *ResellerController) websocket(c *gin.Context) {
	account := a.account(c)
	principal := session.ResellerPrincipal{ID: account.ID, Path: account.Path, Version: account.Version}
	if origin := c.GetHeader("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host != c.Request.Host {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	go func() {
		defer conn.Close()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		done := make(chan struct{})
		go func() {
			defer close(done)
			conn.SetReadLimit(maxMessageSize)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		send := func() error {
			service.ResellerAccessMu.RLock()
			defer service.ResellerAccessMu.RUnlock()
			current := session.CurrentReseller(principal)
			if current == nil {
				return errors.New("reseller access revoked")
			}
			inbounds, err := a.service.Inbounds(current)
			if err != nil {
				return err
			}
			online, last, err := a.service.Online(current)
			if err != nil {
				return err
			}
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			for _, message := range []any{gin.H{"type": "resellerAccess", "payload": gin.H{"canEditInbounds": current.CanEditInbounds}}, gin.H{"type": "inbounds", "payload": inbounds}, gin.H{"type": "traffic", "payload": gin.H{"onlineClients": online, "lastOnlineMap": last}}} {
				if err := conn.WriteJSON(message); err != nil {
					return err
				}
			}
			return nil
		}
		if send() != nil {
			return
		}
		seconds := 0
		lastScope := ""
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if session.CurrentReseller(principal) == nil {
					_ = conn.WriteControl(ws.CloseMessage, ws.FormatCloseMessage(ws.ClosePolicyViolation, "Access changed; please log in again"), time.Now().Add(writeWait))
					return
				}
				seconds++
				current := session.CurrentReseller(principal)
				if current == nil {
					return
				}
				ids, err := a.service.AssignedIDs(current)
				if err != nil {
					return
				}
				scope := fmt.Sprint(current.CanEditInbounds, ids)
				if seconds%5 == 0 || scope != lastScope {
					if send() != nil {
						return
					}
					lastScope = scope
				}
			}
		}
	}()
}
