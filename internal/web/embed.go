package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

// Handler returns an http.Handler that serves the embedded static files.
// It implements SPA fallback: any request for a path that does not match
// an existing file in the embedded filesystem is served index.html,
// allowing the client-side router to handle it.
func Handler() http.Handler {
	// Strip the "static" prefix so files are served from root.
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("web: failed to create sub-filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the URL path.
		upath := r.URL.Path
		if !strings.HasPrefix(upath, "/") {
			upath = "/" + upath
		}
		upath = path.Clean(upath)

		// Try to open the requested file in the embedded FS.
		// If it exists, let the standard file server handle it
		// (it sets Content-Type correctly via mime sniffing / extension).
		if upath == "/" {
			// Root path — serve index.html directly.
			fileServer.ServeHTTP(w, r)
			return
		}

		// Strip leading slash for fs.Open.
		name := strings.TrimPrefix(upath, "/")
		f, err := sub.(fs.ReadFileFS).ReadFile(name)
		if err == nil && f != nil {
			// File exists — let the file server handle it with proper
			// Content-Type detection, caching headers, etc.
			fileServer.ServeHTTP(w, r)
			return
		}

		// File not found in embedded FS — SPA fallback: serve index.html.
		// Rewrite the request path so the file server finds index.html.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
