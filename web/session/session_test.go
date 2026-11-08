package session

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestCredentialChangeInvalidatesAllOldCookies(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "sessions.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	var user model.User
	if err := database.GetDB().First(&user).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(sessions.Sessions("test-session", cookie.NewStore([]byte("session-test-signing-key-32-bytes!"))))
	router.GET("/login", func(c *gin.Context) {
		SetLoginUser(c, &user)
		if err := sessions.Default(c).Save(); err != nil {
			t.Fatal(err)
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/private", func(c *gin.Context) {
		if !IsLogin(c) {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusNoContent)
	})
	var cookies []*http.Cookie
	for i := 0; i < 2; i++ {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/login", nil))
		cookies = append(cookies, response.Result().Cookies()[0])
	}
	requestWithCookie := func(cookie *http.Cookie) int {
		request := httptest.NewRequest("GET", "/private", nil)
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response.Code
	}
	for _, cookie := range cookies {
		if requestWithCookie(cookie) != http.StatusNoContent {
			t.Fatal("valid login rejected")
		}
	}
	if err := database.GetDB().Model(&user).Update("password", "a-different-password-hash").Error; err != nil {
		t.Fatal(err)
	}
	for _, cookie := range cookies {
		if requestWithCookie(cookie) != http.StatusUnauthorized {
			t.Fatal("old session remained logged in")
		}
	}
	var current model.User
	database.GetDB().First(&current, user.Id)
	if !UserIsCurrent(&current) {
		t.Fatal("new credentials rejected")
	}
	if UserIsCurrent(&model.User{Id: user.Id, Username: user.Username, Password: "old"}) {
		t.Fatal("old websocket credentials accepted")
	}
}
