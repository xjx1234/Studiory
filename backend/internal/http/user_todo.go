package http

import (
	todoservice "backend/internal/service/todo"
	"backend/pkg/pagination"
	"backend/pkg/request"
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

type UserTodoHandler struct {
	svc todoservice.Service
}

func NewUserTodoHandler(svc todoservice.Service) *UserTodoHandler {
	return &UserTodoHandler{svc: svc}
}

type createTodoReq struct {
	Title string `json:"title" binding:"required,min=1,max=200"`
}

type updateTodoReq struct {
	Title string `json:"title" binding:"required,min=1,max=200"`
	Done  bool   `json:"done"`
}

type todoIDParam struct {
	ID string `uri:"id" binding:"required,uuid"`
}

// List GET /api/v1/user/todos
func (h *UserTodoHandler) List(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	page := pagination.ParseQuery(c)
	list, e := h.svc.List(c.Request.Context(), userID, page)
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, list)
}

// Create POST /api/v1/user/todos
func (h *UserTodoHandler) Create(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	var req createTodoReq
	if !request.Bind(c, &req) {
		return
	}

	item, e := h.svc.Create(c.Request.Context(), userID, &todoservice.CreateInput{
		Title: req.Title,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, item)
}

// Get GET /api/v1/user/todos/:id
func (h *UserTodoHandler) Get(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	var param todoIDParam
	if !request.BindURI(c, &param) {
		return
	}

	item, e := h.svc.Get(c.Request.Context(), userID, param.ID)
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, item)
}

// Update PATCH /api/v1/user/todos/:id
func (h *UserTodoHandler) Update(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	var param todoIDParam
	if !request.BindURI(c, &param) {
		return
	}

	var req updateTodoReq
	if !request.Bind(c, &req) {
		return
	}

	item, e := h.svc.Update(c.Request.Context(), userID, param.ID, &todoservice.UpdateInput{
		Title: req.Title,
		Done:  req.Done,
	})
	if e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, item)
}

// Delete DELETE /api/v1/user/todos/:id
func (h *UserTodoHandler) Delete(c *gin.Context) {
	userID, ok := mustUserID(c)
	if !ok {
		return
	}

	var param todoIDParam
	if !request.BindURI(c, &param) {
		return
	}

	if e := h.svc.Delete(c.Request.Context(), userID, param.ID); e != nil {
		resp.Fail(c, e)
		return
	}
	resp.OK(c, nil)
}
