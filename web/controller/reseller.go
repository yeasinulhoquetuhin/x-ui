package controller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
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
	c.Set("base_path", root+account.Path+"/")
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

func (a *ResellerController) account(c *gin.Context) *model.ResellerAccess {
	return c.MustGet("reseller_account").(*model.ResellerAccess)
}

func (a *ResellerController) authenticate(c *gin.Context) {
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
	if c.ShouldBind(&form) != nil || len(form.Password) > 72 || form.Username != account.Username || !crypto.CheckPasswordHash(account.PasswordHash, form.Password) {
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
	a.audit(c, "login", "")
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
	jsonObj(c, defaults, nil)
}

func (a *ResellerController) list(c *gin.Context) {
	inbound, err := a.service.Inbound(a.account(c).InboundID)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	jsonObj(c, []*model.Inbound{inbound}, nil)
}

func (a *ResellerController) onlines(c *gin.Context) {
	online, _, err := a.service.Online(a.account(c).InboundID)
	jsonObj(c, online, err)
}
func (a *ResellerController) lastOnline(c *gin.Context) {
	_, last, err := a.service.Online(a.account(c).InboundID)
	jsonObj(c, last, err)
}

func (a *ResellerController) scopeID(c *gin.Context, id int) bool {
	if id != a.account(c).InboundID {
		pureJsonMsg(c, http.StatusForbidden, false, "This inbound is not assigned to your account")
		return false
	}
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
	if !a.service.OwnsEmail(a.account(c).InboundID, c.Param("email")) {
		pureJsonMsg(c, http.StatusForbidden, false, "Client is not assigned to your account")
		return false
	}
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
	if replacing != "" && !a.ownsClient(c, replacing) {
		return
	}
	data, err := a.service.ClientInput(form.ID, form.Settings, replacing)
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
			a.audit(c, action, client.Email)
		}
	}
	jsonMsg(c, "Client saved", err)
}

func (a *ResellerController) addClient(c *gin.Context)    { a.mutate(c, "") }
func (a *ResellerController) updateClient(c *gin.Context) { a.mutate(c, c.Param("clientId")) }

func (a *ResellerController) ownsClient(c *gin.Context, id string) bool {
	inbound, err := a.inbound.GetInbound(a.account(c).InboundID)
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
	if !a.scopePath(c) || !a.ownsClient(c, c.Param("clientId")) {
		return
	}
	restart, err := a.inbound.DelInboundClient(a.account(c).InboundID, c.Param("clientId"))
	if err == nil {
		if restart {
			a.core.SetToNeedRestart()
		}
		a.audit(c, "client.delete", "client")
	}
	jsonMsg(c, "Client deleted", err)
}
func (a *ResellerController) resetClient(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.scopePath(c) || !a.scopeEmail(c) {
		return
	}
	restart, err := a.inbound.ResetClientTraffic(a.account(c).InboundID, c.Param("email"))
	if err == nil {
		if restart {
			a.core.SetToNeedRestart()
		}
		a.audit(c, "client.reset", c.Param("email"))
	}
	jsonMsg(c, "Client traffic reset", err)
}
func (a *ResellerController) resetClients(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.scopePath(c) {
		return
	}
	err := a.inbound.ResetAllClientTraffics(a.account(c).InboundID)
	if err == nil {
		a.core.SetToNeedRestart()
		a.audit(c, "clients.reset", "all")
	}
	jsonMsg(c, "Client traffic reset", err)
}
func (a *ResellerController) deleteDepleted(c *gin.Context) {
	resellerClientWriteMu.Lock()
	defer resellerClientWriteMu.Unlock()
	if !a.scopePath(c) {
		return
	}
	err := a.inbound.DelDepletedClients(a.account(c).InboundID)
	if err == nil {
		a.audit(c, "clients.deleteDepleted", "depleted")
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

func (a *ResellerController) audit(c *gin.Context, action, target string) {
	if err := a.service.Audit(a.account(c), action, target, resellerPeer(c)); err != nil {
		logger.Warningf("Reseller audit: %v", err)
	}
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
			if session.CurrentReseller(principal) == nil {
				return errors.New("reseller access revoked")
			}
			inbound, err := a.service.Inbound(account.InboundID)
			if err != nil {
				return err
			}
			online, last, err := a.service.Online(account.InboundID)
			if err != nil {
				return err
			}
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			for _, message := range []any{gin.H{"type": "inbounds", "payload": []*model.Inbound{inbound}}, gin.H{"type": "traffic", "payload": gin.H{"onlineClients": online, "lastOnlineMap": last}}} {
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
				if seconds%5 == 0 {
					if send() != nil {
						return
					}
				}
			}
		}
	}()
}
