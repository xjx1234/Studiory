package http

import (
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

func registerUserEnglishWordRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/english-word")

	// GET /api/v1/user/english-word/modes
	g.GET("/modes", func(c *gin.Context) {
		// TODO: 调用 internal/user.EnglishWordService.ListModes
		resp.OK(c, nil)
	})

	// POST /api/v1/user/english-word/sessions
	g.POST("/sessions", func(c *gin.Context) {
		// TODO: 调用 EnglishWordService.CreateSession
		resp.OK(c, nil)
	})

	// POST /api/v1/user/english-word/sessions/:sessionId/submit
	g.POST("/sessions/:sessionId/submit", func(c *gin.Context) {
		// TODO: 调用 EnglishWordService.SubmitAnswers
		resp.OK(c, gin.H{"session_id": c.Param("sessionId")})
	})

	// GET /api/v1/user/english-word/sessions/:sessionId
	g.GET("/sessions/:sessionId", func(c *gin.Context) {
		// TODO: 调用 EnglishWordService.GetSessionResult
		resp.OK(c, gin.H{"session_id": c.Param("sessionId")})
	})
}
