package dashboard

import (
	"bytes"
	"io/fs"
	"net/http"
	"strconv"
)

const apiBasePlaceholder = "<!--DREAM_API_BASE-->"

type Server struct {
	assets  fs.FS
	apiBase string
}

func New(apiBase string) *Server {
	root := siteRoot()
	sub, err := fs.Sub(webFS, root)
	if err != nil {
		panic("internal/dashboard: embedded " + root + " directory missing: " + err.Error())
	}
	return &Server{assets: sub, apiBase: apiBase}
}

func (s *Server) Handler() http.Handler {
	fileServer := http.FileServer(http.FS(s.assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "/index.html" || path == "" {
			s.serveIndex(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	raw, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		http.Error(w, "dashboard assets missing index.html", http.StatusInternalServerError)
		return
	}

	script := "<script>window.__DREAM_API_BASE__=" + strconv.Quote(s.apiBase) + ";</script>"
	out := bytes.Replace(raw, []byte(apiBasePlaceholder), []byte(script), 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

func Mount(prefix string, apiBase string) http.Handler {
	return http.StripPrefix(prefix, New(apiBase).Handler())
}
