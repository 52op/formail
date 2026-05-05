//go:build !embed

package formail

import "io/fs"

// WebFS is nil in standard builds. Web/static files are served from disk.
var WebFS fs.FS
