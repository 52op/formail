//go:build !embed

package main

import "github.com/gin-gonic/gin"

func serveStatic(r *gin.Engine) {
	r.Static("/assets", "./web/static")
}

func servePage(c *gin.Context, path string) {
	c.File("./web/static/pages" + path)
}
