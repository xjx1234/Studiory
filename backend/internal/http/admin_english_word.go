package http

import (
	"backend/pkg/resp"

	"github.com/gin-gonic/gin"
)

func registerAdminEnglishWordRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/english-word")

	// GET /api/v1/admin/english-word/word-sets
	g.GET("/word-sets", func(c *gin.Context) {
		// TODO: 调用 EnglishWordAdminService.ListWordSets
		resp.OK(c, nil)
	})

	// POST /api/v1/admin/english-word/word-sets
	g.POST("/word-sets", func(c *gin.Context) {
		// TODO: 调用 EnglishWordAdminService.SaveWordSet
		resp.OK(c, nil)
	})

	// GET /api/v1/admin/english-word/word-sets/:wordSetId/words
	g.GET("/word-sets/:wordSetId/words", func(c *gin.Context) {
		// TODO: 调用 EnglishWordAdminService.ListWords
		resp.OK(c, gin.H{"word_set_id": c.Param("wordSetId")})
	})

	// POST /api/v1/admin/english-word/word-sets/:wordSetId/words
	g.POST("/word-sets/:wordSetId/words", func(c *gin.Context) {
		// TODO: 调用 EnglishWordAdminService.SaveWord
		resp.OK(c, gin.H{"word_set_id": c.Param("wordSetId")})
	})
}
