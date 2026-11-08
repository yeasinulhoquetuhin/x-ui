package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"github.com/mhsanaei/3x-ui/v2/web/websocket"
	"github.com/op/go-logging"
)

func TestCredentialChangeClosesOpenWebSocket(t *testing.T) {
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	if err := database.InitDB(filepath.Join(t.TempDir(), "websocket.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	var user model.User
	if err := database.GetDB().First(&user).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(sessions.Sessions("test", cookie.NewStore([]byte("websocket-session-test-key-32-byte"))))
	router.GET("/login", func(c *gin.Context) { session.SetLoginUser(c, &user); _ = sessions.Default(c).Save(); c.Status(204) })
	hub := websocket.NewHub()
	hubDone := make(chan struct{})
	go func() { defer close(hubDone); hub.Run() }()
	defer func() { hub.Stop(); <-hubDone }()
	controller := NewWebSocketController(hub)
	router.GET("/ws", controller.HandleWebSocket)
	server := httptest.NewServer(router)
	defer server.Close()
	resp, err := http.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	header := http.Header{}
	header.Set("Cookie", resp.Cookies()[0].String())
	conn, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := database.GetDB().Model(&user).Update("password", "new-password-hash").Error; err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, _, err = conn.ReadMessage()
	if !ws.IsCloseError(err, ws.ClosePolicyViolation) {
		t.Fatalf("old websocket remained authenticated: %v", err)
	}
}
