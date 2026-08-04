// cmd/codeindex/serve.go
package main

import (
	"codeindex/internal/query"
	"codeindex/internal/webserver"
)

// runServe freshens the index, then serves the read-only headless JSON graph API.
func runServe(root, addr string) error {
	if _, err := query.Fresh(root); err != nil {
		return err
	}
	return webserver.Run(root, addr, version)
}
