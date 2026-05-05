//go:build embed

package main

import (
	"io/fs"
	"net/http"
	"os"

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
	r.StaticFS("/assets", http.FS(sub))
}

func servePage(c *gin.Context, path string) {
	if _, err := os.Stat("./web/static/pages" + path); err == nil {
		c.File("./web/static/pages" + path)
		return
	}
	sub, err := fs.Sub(formail.WebFS, "web")
	if err != nil {
		c.File("./web/static/pages" + path)
		return
	}
	c.FileFromFS("/pages"+path, http.FS(sub))
}
