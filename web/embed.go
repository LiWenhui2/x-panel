package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist
var distribution embed.FS

func Handler() http.Handler {
	root, err := fs.Sub(distribution, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		requested := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if requested != "." && requested != "" {
			if _, err := fs.Stat(root, requested); err == nil {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				files.ServeHTTP(w, r)
				return
			}
		}
		clone := r.Clone(r.Context())
		clone.URL.Path = "/"
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, clone)
	})
}
