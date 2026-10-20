package web

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type staticConfig struct {
	apiPrefix string
}

// StaticOption configures Static.
type StaticOption func(*staticConfig)

// WithAPIPrefix sets the path prefix that is never served as a page: a request
// under it gets 404 NotFound instead of index.html. The default is "api/".
func WithAPIPrefix(prefix string) StaticOption {
	return func(c *staticConfig) { c.apiPrefix = strings.Trim(prefix, "/") + "/" }
}

// Static serves a built frontend from dir (a Vite-style dist directory):
//
//   - a path under /assets/ is a file in dir/assets; a missing one is 404 NotFound;
//   - a path under the API prefix ("api/" by default) is 404 NotFound;
//   - every other path gets dir/index.html, so client-side routes load the app.
//     A missing index.html is 500 InternalError, as in novastar.
//
// Only GET and HEAD are served; other methods get 405 MethodNotAllowed. A
// request path is never joined to dir unchecked: it must pass fs.ValidPath,
// so ".." and absolute paths cannot leave the directory.
func Static(dir string, opts ...StaticOption) http.Handler {
	cfg := staticConfig{apiPrefix: "api/"}
	for _, o := range opts {
		o(&cfg)
	}
	return &static{dir: dir, cfg: cfg}
}

type static struct {
	dir string
	cfg staticConfig
}

func (s *static) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeTagged(w, http.StatusMethodNotAllowed, "MethodNotAllowed")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if s.cfg.apiPrefix != "/" && strings.HasPrefix(p, s.cfg.apiPrefix) {
		writeTagged(w, http.StatusNotFound, "NotFound")
		return
	}
	if rest, ok := strings.CutPrefix(p, "assets/"); ok {
		if !fs.ValidPath(rest) || rest == "." {
			writeTagged(w, http.StatusNotFound, "NotFound")
			return
		}
		s.serveFile(w, r, filepath.Join(s.dir, "assets", filepath.FromSlash(rest)), true)
		return
	}
	s.serveFile(w, r, filepath.Join(s.dir, "index.html"), false)
}

// serveFile answers with the file at path. missingIs404 says whether a missing
// file is a client error (an asset) or a broken build (index.html).
func (s *static) serveFile(w http.ResponseWriter, r *http.Request, path string, missingIs404 bool) {
	missing := func() {
		if missingIs404 {
			writeTagged(w, http.StatusNotFound, "NotFound")
			return
		}
		writeTagged(w, http.StatusInternalServerError, "InternalError")
	}
	f, err := os.Open(path)
	if err != nil {
		missing()
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		missing()
		return
	}
	// ServeContent, not ServeFile: ServeFile rejects ".." and redirects
	// "/index.html", which must not depend on the request path.
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
