package web

import (
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"github.com/nicolasticot/cass/server/internal/auth"
	"github.com/nicolasticot/cass/server/internal/config"
	"github.com/nicolasticot/cass/server/internal/db"
	"github.com/nicolasticot/cass/server/internal/tlsutil"
)

func Run(cfg *config.Config, database *db.DB) error {
	sessions, err := auth.NewSessionStore(cfg.DataDir)
	if err != nil {
		return err
	}

	h := &Handlers{
		DB:       database,
		Sessions: sessions,
		Tmpls:    mustLoadTemplates(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.requireAuth(h.Dashboard))
	mux.HandleFunc("GET /login", h.LoginForm)
	mux.HandleFunc("POST /login", h.LoginSubmit)
	mux.HandleFunc("POST /logout", h.requireAuth(h.Logout))
	mux.HandleFunc("GET /setup", h.SetupForm)
	mux.HandleFunc("POST /setup", h.SetupSubmit)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	certPath, keyPath, fp, err := tlsutil.EnsureCert(cfg.DataDir)
	if err != nil {
		return err
	}
	log.Printf("--------------------------------------------------------------")
	log.Printf("cass-server listening on https://%s", cfg.ListenAddr)
	log.Printf("TLS fingerprint (SHA-256): %s", fp)
	log.Printf("Pin this fingerprint in any client that connects.")
	log.Printf("--------------------------------------------------------------")

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           logMiddleware(mux),
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServeTLS(certPath, keyPath)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
