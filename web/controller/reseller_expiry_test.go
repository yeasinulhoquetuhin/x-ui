package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func TestExpiredResellerCannotLoginOrUseExistingCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := database.InitDB(filepath.Join(t.TempDir(), "expiry.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	var s service.ResellerService
	account, err := s.Save(1, 0, service.ResellerAccessForm{Enabled: true, Path: "expiry", Username: "shop", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	NewResellerController(engine.Group("/reseller"), []byte("expiry-test-secret"))
	req := httptest.NewRequest("POST", "/reseller/expiry/login", strings.NewReader(url.Values{"username": {"shop"}, "password": {"password"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatal(w.Body.String())
	}
	cookies := w.Result().Cookies()
	past := time.Now().Add(-time.Second).UnixMilli()
	if err := database.GetDB().Model(&model.ResellerAccount{}).Where("id = ?", account.ID).Update("expiry_time", past).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/reseller/expiry/login", "/reseller/expiry/panel/api/inbounds/list", "/reseller/expiry/ws"} {
		method := "GET"
		if strings.HasSuffix(path, "/login") {
			method = "POST"
		}
		r := httptest.NewRequest(method, path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "expired") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
}
