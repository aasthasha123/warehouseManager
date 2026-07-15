package api

import (
	"errors"
	"net/http"
	"strings"

	"coldstorage/internal/authutil"
	"coldstorage/internal/store"
)

// POST /api/auth/signup
// Registers a new cold storage company on the platform along with
// its owner (Admin) account — this is the "company registration /
// access platform" entry point.
type signupRequest struct {
	CompanyName string `json:"company_name"`
	AdminName   string `json:"admin_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.CompanyName = strings.TrimSpace(req.CompanyName)
	req.AdminName = strings.TrimSpace(req.AdminName)

	if req.CompanyName == "" || req.AdminName == "" || req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "company_name, admin_name, email are required and password must be at least 8 characters")
		return
	}

	hash, err := authutil.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not process password")
		return
	}

	company, owner, err := s.db.CreateCompanyWithOwner(r.Context(), req.CompanyName, req.AdminName, req.Email, hash)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "an account with this email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create company")
		return
	}

	sess, err := s.db.CreateSession(r.Context(), owner.ID, company.ID, owner.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account created, but login failed — try logging in")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"token":   sess.Token,
		"company": company,
		"user":    owner,
	})
}

// POST /api/auth/login
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	user, err := s.db.FindUserByEmail(r.Context(), req.Email)
	if err != nil || !authutil.VerifyPassword(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	sess, err := s.db.CreateSession(r.Context(), user.ID, user.CompanyID, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not log in, please try again")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": sess.Token,
		"user":  user,
	})
}

// POST /api/auth/logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	_ = s.db.DeleteSession(r.Context(), token)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// POST /api/supervisors  (admin only)
// The way an Admin/Owner adds an In-charge (Supervisor) to their company.
type createSupervisorRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleCreateSupervisor(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)

	var req createSupervisorRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "name, email are required and password must be at least 8 characters")
		return
	}

	hash, err := authutil.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not process password")
		return
	}

	supervisor, err := s.db.CreateSupervisor(r.Context(), sess.CompanyID, req.Name, req.Email, hash)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "an account with this email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create supervisor")
		return
	}

	writeJSON(w, http.StatusCreated, supervisor)
}
