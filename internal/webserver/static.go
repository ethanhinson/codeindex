// internal/webserver/static.go
package webserver

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var distFS embed.FS

// staticHandler serves the embedded SPA assets from dist/.
func staticHandler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // dist is embedded at build time; this cannot fail in a built binary
	}
	return http.FileServer(http.FS(sub))
}
