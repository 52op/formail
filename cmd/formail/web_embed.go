//go:build embed

package main

import (
	"io/fs"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"formail"

	"github.com/gin-gonic/gin"
)

func serveStatic(r *gin.Engine) {
	if _, err := os.Stat("./web/static"); err == nil {
		r.Static("/assets", "./web/static")
		return
	}
	sub, err := fs.Sub(formail.WebFS, "web/static")
	if err != nil {
		r.Static("/assets", "./web/static")
		return
	}
	r.GET("/assets/*filepath", func(c *gin.Context) {
		fp := strings.TrimPrefix(c.Param("filepath"), "/")
		data, err := fs.ReadFile(sub, fp)
		if err != nil {
			c.Status(404)
			return
		}
		ct := mime.TypeByExtension(filepath.Ext(fp))
		if ct == "" {
			ct = "application/octet-stream"
		}
		c.Data(200, ct, data)
	})
}

func servePage(c *gin.Context, path string) {
	if _, err := os.Stat("./web/static/pages" + path); err == nil {
		c.File("./web/static/pages" + path)
		return
	}
	if formail.WebFS == nil {
		c.File("./web/static/pages" + path)
		return
	}
	sub, err := fs.Sub(formail.WebFS, "web/static")
	if err != nil {
		log.Printf("[servePage] fs.Sub error: %v", err)
		c.File("./web/static/pages" + path)
		return
	}
	data, err := fs.ReadFile(sub, "pages"+path)
	if err != nil {
		log.Printf("[servePage] fs.ReadFile error: %v", err)
		c.File("./web/static/pages" + path)
		return
	}
	c.Data(200, "text/html; charset=utf-8", data)
}
