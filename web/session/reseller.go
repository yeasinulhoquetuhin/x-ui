package session

import (
	"encoding/gob"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

type ResellerPrincipal struct {
	ID      int
	Version string
	Path    string
}

func init() { gob.Register(ResellerPrincipal{}) }

func SetReseller(c *gin.Context, account *model.ResellerAccess) {
	sessions.Default(c).Set("RESELLER", ResellerPrincipal{ID: account.ID, Version: account.Version, Path: account.Path})
}

func CurrentReseller(principal ResellerPrincipal) *model.ResellerAccess {
	if database.GetDB() == nil {
		return nil
	}
	var account model.ResellerAccess
	if database.GetDB().Where("id = ? AND enabled = ? AND version = ? AND path = ?", principal.ID, true, principal.Version, principal.Path).
		Where("inbound_id IN (SELECT id FROM inbounds)").First(&account).Error != nil {
		return nil
	}
	return &account
}

func GetReseller(c *gin.Context) *model.ResellerAccess {
	principal, ok := sessions.Default(c).Get("RESELLER").(ResellerPrincipal)
	if !ok || principal.Path != c.Param("resellerPath") {
		return nil
	}
	return CurrentReseller(principal)
}
