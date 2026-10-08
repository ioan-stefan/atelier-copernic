package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Page carries the fields every template's layout needs.
type Page struct {
	Title       string
	Description string
	Section     string // highlights the current navigation item
}

// Renderer holds one parsed template set per page. Each set is the shared
// layout and partials plus that page's own "main" block, so page blocks
// never collide.
type Renderer struct {
	pages map[string]*template.Template
	log   *slog.Logger
}

var pageNames = []string{"vault", "ledger", "entry", "commission", "review", "received", "error"}

func NewRenderer(fsys fs.FS, assets *AssetStore, log *slog.Logger) (*Renderer, error) {
	funcs := template.FuncMap{
		"asset":     assets.URL,
		"lat":       formatLatitude,
		"chf":       formatCHF,
		"years":     formatYears,
		"drift":     formatDrift,
		"thousands": func(n int) string { return groupThousands(int64(n)) },
		"year":      func() int { return time.Now().Year() },
		"stageName": func(i int) string { return stageNames[i] },
	}
	r := &Renderer{pages: make(map[string]*template.Template, len(pageNames)), log: log}
	for _, name := range pageNames {
		t, err := template.New(name).Funcs(funcs).ParseFS(fsys,
			"templates/layout.html", "templates/partials.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		r.pages[name] = t
	}
	return r, nil
}

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// Render executes into a buffer first, so a template error becomes a clean
// 500 instead of a half-written page, then writes compressed when accepted.
func (rd *Renderer) Render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	t, ok := rd.pages[page]
	if !ok {
		rd.log.Error("unknown page", "page", page)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		rd.log.Error("render", "page", page, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "no-cache")
	}
	writeBody(w, r, status, buf.Bytes())
}

func writeBody(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	h := w.Header()
	h.Add("Vary", "Accept-Encoding")
	if len(body) > 1024 && acceptsGzip(r) {
		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)
		var out bytes.Buffer
		gz.Reset(&out)
		_, _ = gz.Write(body)
		_ = gz.Close()
		h.Set("Content-Encoding", "gzip")
		body = out.Bytes()
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.TrimSpace(name) != "gzip" {
			continue
		}
		q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
	}
	return false
}

// asset is a static file held in memory with a precompressed copy.
type asset struct {
	body  []byte
	gz    []byte
	ctype string
	etag  string
}

// AssetStore serves the embedded static directory with fingerprinted URLs,
// so stylesheets can be cached as immutable.
type AssetStore struct {
	files map[string]asset
}

var assetTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".svg":   "image/svg+xml",
	".txt":   "text/plain; charset=utf-8",
	".woff2": "font/woff2",
	".png":   "image/png",
	".ico":   "image/x-icon",
}

func NewAssetStore(fsys fs.FS) (*AssetStore, error) {
	s := &AssetStore{files: make(map[string]asset)}
	err := fs.WalkDir(fsys, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		ctype, ok := assetTypes[path.Ext(p)]
		if !ok {
			ctype = "application/octet-stream"
		}
		sum := sha256.Sum256(body)
		a := asset{body: body, ctype: ctype, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
		if strings.HasPrefix(ctype, "text/") || ctype == "image/svg+xml" {
			var buf bytes.Buffer
			gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			_, _ = gz.Write(body)
			_ = gz.Close()
			a.gz = buf.Bytes()
		}
		s.files[strings.TrimPrefix(p, "static/")] = a
		return nil
	})
	return s, err
}

// URL returns a cache-busting URL for a static file.
func (s *AssetStore) URL(name string) string {
	a, ok := s.files[name]
	if !ok {
		return "/static/" + name
	}
	return "/static/" + name + "?v=" + strings.Trim(a.etag, `"`)
}

func (s *AssetStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a, ok := s.files[r.PathValue("file")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", a.ctype)
	h.Set("ETag", a.etag)
	h.Set("Vary", "Accept-Encoding")
	if r.URL.Query().Get("v") != "" {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=3600")
	}
	if r.Header.Get("If-None-Match") == a.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := a.body
	if a.gz != nil && acceptsGzip(r) {
		h.Set("Content-Encoding", "gzip")
		body = a.gz
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}
