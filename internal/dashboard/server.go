// Package dashboard is a minimal, HTTP-Basic-Auth-protected read-only view
// over the applications SQLite store — deliberately not exposed over SSH,
// since only the company's recruiters should see it.
package dashboard

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/sharadregoti/sshire/internal/store"
)

type Server struct {
	store    *store.Store
	user     string
	password string
	mux      *http.ServeMux
}

func New(s *store.Store, user, password string) *Server {
	srv := &Server{store: s, user: user, password: password, mux: http.NewServeMux()}
	srv.mux.HandleFunc("/", srv.handleList)
	srv.mux.HandleFunc("/applications/", srv.handleDetail)
	return srv
}

func (s *Server) ListenAndServe(addr string) error {
	log.Printf("dashboard listening on %s", addr)
	return http.ListenAndServe(addr, s.withAuth(s.mux))
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.user == "" && s.password == "" {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.user || pass != s.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="sshire dashboard"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var listTmpl = template.Must(template.New("list").Parse(`<!doctype html>
<html><head><title>Applications</title>
<style>
body{font-family:monospace;background:#111;color:#eee;padding:2rem}
table{border-collapse:collapse;width:100%}
td,th{border-bottom:1px solid #333;padding:.5rem;text-align:left}
a{color:#7dd3fc}
</style></head><body>
<h1>Applications</h1>
<table>
<tr><th>ID</th><th>Job</th><th>Name</th><th>Email</th><th>Submitted</th></tr>
{{range .}}
<tr>
<td><a href="/applications/{{.ID}}">{{.ID}}</a></td>
<td>{{.JobTitle}}</td>
<td>{{.Name}}</td>
<td>{{.Email}}</td>
<td>{{.SubmittedAt.Format "2006-01-02 15:04"}}</td>
</tr>
{{end}}
</table>
</body></html>`))

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	records, err := s.store.List(r.Context(), 200, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := listTmpl.Execute(w, records); err != nil {
		log.Printf("render list: %v", err)
	}
}

var detailTmpl = template.Must(template.New("detail").Parse(`<!doctype html>
<html><head><title>Application #{{.ID}}</title>
<style>body{font-family:monospace;background:#111;color:#eee;padding:2rem}
dt{color:#7dd3fc;margin-top:1rem}dd{margin:0;white-space:pre-wrap}</style>
</head><body>
<p><a href="/">&larr; back</a></p>
<h1>{{.Name}} &mdash; {{.JobTitle}}</h1>
<dl>
<dt>Email</dt><dd>{{.Email}}</dd>
<dt>Resume / links</dt><dd>{{.ResumeLink}}</dd>
<dt>Message</dt><dd>{{.Message}}</dd>
<dt>Submitted</dt><dd>{{.SubmittedAt}}</dd>
<dt>From</dt><dd>{{.RemoteAddr}}</dd>
</dl>
</body></html>`))

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/applications/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rec, err := s.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, fmt.Sprintf("not found: %v", err), http.StatusNotFound)
		return
	}
	if err := detailTmpl.Execute(w, rec); err != nil {
		log.Printf("render detail: %v", err)
	}
}
