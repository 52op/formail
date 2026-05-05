//go:build embed

package formail

import (
	"embed"
	"io/fs"
)

//go:embed all:web
var embeddedStatic embed.FS

// WebFS provides access to embedded web static files. Nil when not built with embed tag.
var WebFS fs.FS = embeddedStatic
