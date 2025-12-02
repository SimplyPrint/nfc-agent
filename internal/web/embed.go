package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Handler returns an HTTP handler that serves the embedded static files.
// The status page is served at the root path.
func Handler() http.Handler {
	// Get the static subdirectory
	fsys, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}

	return http.FileServer(http.FS(fsys))
}

// HandlerFunc returns an http.HandlerFunc that serves the embedded static files.
func HandlerFunc() http.HandlerFunc {
	handler := Handler()
	return func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}
}
