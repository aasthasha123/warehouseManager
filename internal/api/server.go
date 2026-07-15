package api

import (
	"io/fs"
	"net/http"

	"coldstorage/internal/store"
)

type Server struct {
	db      *store.DB
	webRoot fs.FS
}

// NewServer wires the store and, if provided, a filesystem to serve
// the frontend from at "/". Pass a nil webFS to run API-only.
func NewServer(db *store.DB, webFS fs.FS) *Server {
	return &Server{db: db, webRoot: webFS}
}

// Routes builds the full HTTP route table. Go 1.22's ServeMux
// supports "METHOD /path/{param}" patterns natively, so no router
// dependency is needed.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /api/health", s.handleHealth)

	// Auth — signup creates a Company + its owner Admin in one call.
	mux.HandleFunc("POST /api/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.handleLogout))

	// Company & team management (admin only for adding/listing supervisors)
	mux.HandleFunc("GET /api/me", s.requireAuth(s.handleMe))
	mux.HandleFunc("GET /api/company/me", s.requireAuth(s.handleCompanyMe))
	mux.HandleFunc("POST /api/supervisors", s.requireAuth(s.requireRole(store.RoleAdmin)(s.handleCreateSupervisor)))
	mux.HandleFunc("GET /api/supervisors", s.requireAuth(s.requireRole(store.RoleAdmin)(s.handleListSupervisors)))

	// Farmers — onboarding (register) and lookup. Both admin and
	// supervisor can do this; supervisors do it day-to-day.
	mux.HandleFunc("POST /api/farmers", s.requireAuth(s.handleCreateFarmer))
	mux.HandleFunc("GET /api/farmers", s.requireAuth(s.handleListFarmers))
	mux.HandleFunc("GET /api/farmers/{id}", s.requireAuth(s.handleGetFarmer))
	// This is the endpoint the frontend hits right after scanning a farmer's QR.
	mux.HandleFunc("GET /api/farmers/scan/{token}", s.requireAuth(s.handleScanFarmer))

	// Lots — every scan-and-add appends here, tied to the farmer's ID.
	mux.HandleFunc("POST /api/farmers/{id}/lots", s.requireAuth(s.handleAddLot))
	// Editing a lot (e.g. a farmer taking goods back) — logged automatically.
	mux.HandleFunc("PATCH /api/lots/{id}", s.requireAuth(s.handleUpdateLot))

	// Logs — every lot edit under the company, newest first.
	mux.HandleFunc("GET /api/logs", s.requireAuth(s.handleListLogs))

	// Frontend — served at "/" if a web root was provided (see cmd/server/main.go).
	if s.webRoot != nil {
		mux.Handle("/", http.FileServerFS(s.webRoot))
	}

	return withCORS(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// withCORS keeps this permissive since the frontend may be served
// from a different origin during development (e.g. opened as a
// plain file, or hosted separately). Lock this down to your real
// frontend origin before going to production.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
