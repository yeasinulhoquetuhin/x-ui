package controller

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
)

type ResellerAdminController struct{ service service.ResellerService }

func NewResellerAdminController(g *gin.RouterGroup) *ResellerAdminController {
	a := &ResellerAdminController{}
	g.GET("", a.list)
	g.POST("", a.create)
	g.POST("/:id", a.update)
	g.POST("/:id/delete", a.delete)
	g.GET("/:id/audit", a.audit)
	return a
}

func (a *ResellerAdminController) id(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		pureJsonMsg(c, 400, false, "Invalid reseller ID")
		return 0, false
	}
	if _, err := a.service.Get(session.GetLoginUser(c).Id, id); err != nil {
		pureJsonMsg(c, 404, false, "Reseller not found")
		return 0, false
	}
	return id, true
}

func (a *ResellerAdminController) list(c *gin.Context) {
	accounts, err := a.service.List(session.GetLoginUser(c).Id)
	jsonObj(c, accounts, err)
}

func (a *ResellerAdminController) save(c *gin.Context, id int) {
	var form service.ResellerAccessForm
	if err := c.ShouldBind(&form); err != nil {
		jsonObj(c, nil, err)
		return
	}
	account, err := a.service.Save(session.GetLoginUser(c).Id, id, form)
	if err != nil {
		jsonObj(c, nil, err)
		return
	}
	jsonMsgObj(c, "Reseller saved", account, nil)
}

func (a *ResellerAdminController) create(c *gin.Context) { a.save(c, 0) }
func (a *ResellerAdminController) update(c *gin.Context) {
	if id, ok := a.id(c); ok {
		a.save(c, id)
	}
}
func (a *ResellerAdminController) delete(c *gin.Context) {
	if id, ok := a.id(c); ok {
		jsonMsg(c, "Reseller deleted", a.service.Delete(session.GetLoginUser(c).Id, id))
	}
}
func (a *ResellerAdminController) audit(c *gin.Context) {
	if id, ok := a.id(c); ok {
		rows, err := a.service.AuditList(id)
		jsonObj(c, rows, err)
	}
}
