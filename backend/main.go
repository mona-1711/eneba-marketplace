package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type App struct {
	db     *sql.DB
	search *SearchIndex
	secure bool // set Secure flag on cookies (production / HTTPS)
}

func main() {
	dbPath := env("DB_PATH", "marketplace.db")
	db, err := openDB(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	idx, err := buildIndex(db)
	if err != nil {
		log.Fatal(err)
	}
	app := &App{db: db, search: idx, secure: os.Getenv("COOKIE_SECURE") == "1"}

	mux := app.routes()

	// Serve the built React app (single deployable: API + SPA on one origin,
	// which also keeps the session cookie first-party).
	staticDir := env("STATIC_DIR", "../frontend/dist")
	if st, err := os.Stat(staticDir); err == nil && st.IsDir() {
		mux.Handle("/", spaHandler(staticDir))
		log.Printf("serving frontend from %s", staticDir)
	}

	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: withLogging(withCORS(mux)), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func (a *App) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })

	// Homepage composition + one endpoint per block
	mux.HandleFunc("GET /api/home", a.handleHome)
	mux.HandleFunc("GET /api/blocks/banners", a.blockBanners)
	mux.HandleFunc("GET /api/blocks/categories", a.blockCategories)
	mux.HandleFunc("GET /api/blocks/bestsellers", a.blockBestsellers)
	mux.HandleFunc("GET /api/blocks/deals", a.blockDeals)
	mux.HandleFunc("GET /api/blocks/new-releases", a.blockNewReleases)
	mux.HandleFunc("GET /api/blocks/budget", a.blockBudget)

	// Catalog
	mux.HandleFunc("GET /api/products", a.handleProducts)
	mux.HandleFunc("GET /api/products/{slug}", a.handleProduct)
	mux.HandleFunc("GET /api/facets", a.handleFacets)
	mux.HandleFunc("GET /api/search/suggest", a.handleSuggest)
	mux.HandleFunc("GET /api/covers/{slug}", a.handleCover)
	mux.HandleFunc("POST /api/cart/quote", a.handleCartQuote)

	// Auth
	mux.HandleFunc("POST /api/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/auth/me", a.handleMe)

	// Signed-in only
	mux.HandleFunc("POST /api/checkout", a.requireAuth(a.handleCheckout))
	mux.HandleFunc("GET /api/orders", a.requireAuth(a.handleOrders))
	mux.HandleFunc("GET /api/orders/{id}", a.requireAuth(a.handleOrder))
	return mux
}

func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	})
}

// withCORS allows the Vite dev server (or a separately hosted frontend) to call the API with cookies.
func withCORS(next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range strings.Split(env("CORS_ORIGINS", "http://localhost:5173"), ",") {
		allowed[strings.TrimSpace(o)] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); allowed[o] {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %s", r.Method, r.URL.RequestURI(), time.Since(start).Round(time.Microsecond))
		}
	})
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
