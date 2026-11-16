package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/op/go-logging"
)

func TestResellerSessionScopeAndRevocation(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	t.Cleanup(logger.CloseLogger)
	gin.SetMode(gin.TestMode)
	if err := database.InitDB(filepath.Join(t.TempDir(), "reseller.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	for _, tag := range []string{"one", "two"} {
		if err := database.GetDB().Create(&model.Inbound{UserId: 1, Tag: tag, Protocol: model.VLESS, Settings: `{"clients":[],"decryption":"none"}`, StreamSettings: `{"network":"tcp"}`}).Error; err != nil {
			t.Fatal(err)
		}
	}
	var reseller service.ResellerService
	first, err := reseller.Save(1, 0, service.ResellerAccessForm{Enabled: true, Path: "shop-one", Username: "shop", Password: "shop-password", InboundIDs: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reseller.Save(1, 0, service.ResellerAccessForm{Enabled: true, Path: "shop-two", Username: "shop", Password: "other-password", InboundIDs: []int{2}})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("test-admin-session-secret-32bytes"))), func(c *gin.Context) { c.Set("base_path", "/test/") })
	group := engine.Group("/test")
	NewIndexController(group)
	adminAPI := group.Group("/panel/api/inbounds")
	adminAPI.Use((&APIController{}).checkAPIAuth)
	NewInboundController(adminAPI)
	NewResellerController(engine.Group("/reseller"), []byte("reseller-test-secret"))
	request := func(method, path string, data url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(data.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		writer := httptest.NewRecorder()
		engine.ServeHTTP(writer, req)
		return writer
	}
	login := request("POST", "/reseller/shop-one/login", url.Values{"username": {"shop"}, "password": {"shop-password"}}, nil)
	if login.Code != 200 || !strings.Contains(login.Body.String(), `"success":true`) {
		t.Fatal(login.Code, login.Body.String())
	}
	var auth *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "tdz-reseller" {
			auth = cookie
		}
	}
	if auth == nil || auth.Path != "/reseller/shop-one/" || !auth.HttpOnly {
		t.Fatal("missing scoped reseller cookie")
	}
	passwordURL := "/reseller/shop-one/panel/api/account/password"
	passwordForm := url.Values{"currentPassword": {"shop-password"}, "newPassword": {"new-password"}, "confirmPassword": {"new-password"}}
	if response := request("POST", passwordURL, passwordForm, nil); response.Code != 401 {
		t.Fatal("anonymous password write accepted", response.Code)
	}
	if response := request("POST", passwordURL, passwordForm, auth); response.Code != 403 {
		t.Fatal("default password permission not enforced", response.Code)
	}
	if err := database.GetDB().Model(&model.ResellerAccount{}).Where("id = ?", first.ID).Update("can_edit_password", true).Error; err != nil {
		t.Fatal(err)
	}
	passwordForm.Set("currentPassword", "wrong")
	if response := request("POST", passwordURL, passwordForm, auth); strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatal("wrong current password accepted")
	}
	passwordForm.Set("currentPassword", "shop-password")
	if response := request("POST", passwordURL, passwordForm, auth); response.Code != 200 || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatal("password change failed", response.Code, response.Body.String())
	}
	if response := request("GET", "/reseller/shop-one/panel/api/inbounds/list", nil, auth); response.Code != 401 {
		t.Fatal("password change did not revoke session", response.Code)
	}
	login = request("POST", "/reseller/shop-one/login", url.Values{"username": {"shop"}, "password": {"new-password"}}, nil)
	if !strings.Contains(login.Body.String(), `"success":true`) {
		t.Fatal("new password login failed", login.Body.String())
	}
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == "tdz-reseller" {
			auth = cookie
		}
	}
	list := request("GET", "/reseller/shop-one/panel/api/inbounds/list", nil, auth)
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"tag":"one"`) || strings.Contains(list.Body.String(), `"tag":"two"`) {
		t.Fatal("inbound scope wrong", list.Body.String())
	}
	for _, path := range []string{"/test/shop-one/panel/api/inbounds/list", "/reseller/shop-two/panel/api/inbounds/list", "/test/panel/api/inbounds/list", "/reseller/shop-one/panel/api/inbounds/update/1", "/reseller/shop-one/panel/setting/update", "/reseller/shop-one/panel/api/server/getDb"} {
		response := request("GET", path, nil, auth)
		if response.Code == 200 {
			t.Fatal("restricted path available", path)
		}
	}
	if response := request("POST", "/reseller/shop-one/panel/api/inbounds/addClient", url.Values{"id": {"2"}, "settings": {`{"clients":[]}`}}, auth); response.Code != 403 {
		t.Fatal("foreign inbound write not denied", response.Code)
	}
	for _, path := range []string{"update/1", "toggle/1"} {
		if response := request("POST", "/reseller/shop-one/panel/api/inbounds/"+path, url.Values{"enabled": {"false"}}, auth); response.Code != 403 {
			t.Fatal("default inbound edit permission was not enforced", path, response.Code)
		}
	}
	_, err = reseller.Save(1, first.ID, service.ResellerAccessForm{Enabled: true, Path: first.Path, Username: first.Username, InboundIDs: []int{1, 2}, CanEditInbounds: true})
	if err != nil {
		t.Fatal(err)
	}
	if response := request("GET", "/reseller/shop-one/panel/api/inbounds/list", nil, auth); response.Code != 200 || !strings.Contains(response.Body.String(), `"tag":"two"`) {
		t.Fatal("assignment addition did not apply to current session", response.Body.String())
	}
	_, err = reseller.Save(1, first.ID, service.ResellerAccessForm{Enabled: true, Path: first.Path, Username: first.Username})
	if err != nil {
		t.Fatal(err)
	}
	if response := request("GET", "/reseller/shop-one/panel/api/inbounds/list", nil, auth); response.Code != 200 || !strings.Contains(response.Body.String(), `"obj":[]`) {
		t.Fatal("zero-assignment session failed", response.Body.String())
	}
	if response := request("POST", "/reseller/shop-one/panel/api/inbounds/addClient", url.Values{"id": {"1"}}, auth); response.Code != 403 {
		t.Fatal("removed inbound remained writable", response.Code)
	}
	_, err = reseller.Save(1, first.ID, service.ResellerAccessForm{Enabled: true, Path: first.Path, Username: first.Username, Password: "changed-password", InboundIDs: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if response := request("GET", "/reseller/shop-one/panel/api/inbounds/list", nil, auth); response.Code != 401 {
		t.Fatal("old reseller session was not revoked", response.Code)
	}
}
