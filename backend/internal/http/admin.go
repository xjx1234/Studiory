package http

import (
	"backend/internal/repo"
	adminservice "backend/internal/service/admin"
	"backend/pkg/pagination"
	"backend/pkg/request"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

type AdminUserHandler struct {
	deps *Deps
}

func registerAdminRoutes(rg *gin.RouterGroup, deps *Deps) {
	rg.GET("/ping", func(c *gin.Context) {
		resp.OK(c, gin.H{"status": "ok", "scope": repo.RoleAdmin})
	})

	h := &AdminUserHandler{deps: deps}
	users := rg.Group("/users")
	users.GET("", h.List)
	users.GET("/:id", h.Get)
	users.PATCH("/:id/role", h.UpdateRole)
	users.PATCH("/:id/status", h.SetStatus)
}

type adminUserIDParam struct {
	ID string `uri:"id" binding:"required,uuid"`
}

type updateRoleReq struct {
	Role string `json:"role" binding:"required,oneof=admin user"`
}

type setStatusReq struct {
	Status string `json:"status" binding:"required,oneof=active disabled"`
}

// List GET /api/v1/admin/users?page=&page_size=&keyword=&status=
func (h *AdminUserHandler) List(c *gin.Context) {
	page := pagination.ParseQuery(c)
	in := adminservice.ListInput{
		Keyword: c.Query("keyword"),
		Status:  c.Query("status"),
	}

	list, e := h.deps.AdminService.ListUsers(c.Request.Context(), in, page)
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, list)
}

// Get GET /api/v1/admin/users/:id
func (h *AdminUserHandler) Get(c *gin.Context) {
	var param adminUserIDParam
	if !request.BindURI(c, &param) {
		return
	}

	item, e := h.deps.AdminService.GetUser(c.Request.Context(), param.ID)
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, item)
}

// UpdateRole PATCH /api/v1/admin/users/:id/role
func (h *AdminUserHandler) UpdateRole(c *gin.Context) {
	actingUserID, ok := mustUserID(c)
	if !ok {
		return
	}

	var param adminUserIDParam
	if !request.BindURI(c, &param) {
		return
	}

	var req updateRoleReq
	if !request.Bind(c, &req) {
		return
	}

	item, e := h.deps.AdminService.UpdateRole(c.Request.Context(), actingUserID, param.ID, req.Role)
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, item)
}

// SetStatus PATCH /api/v1/admin/users/:id/status
func (h *AdminUserHandler) SetStatus(c *gin.Context) {
	actingUserID, ok := mustUserID(c)
	if !ok {
		return
	}

	var param adminUserIDParam
	if !request.BindURI(c, &param) {
		return
	}

	var req setStatusReq
	if !request.Bind(c, &req) {
		return
	}

	item, e := h.deps.AdminService.SetStatus(c.Request.Context(), actingUserID, param.ID, req.Status)
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, item)
}
