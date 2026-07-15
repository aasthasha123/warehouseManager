package api

import (
	"context"
	"net/http"
	"strings"

	"coldstorage/internal/store"
)

type ctxKey string

const sessionCtxKey ctxKey = "session"

// requireAuth validates the "Authorization: Bearer <token>" header
// against an active session and attaches it to the request context.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		sess, err := s.db.GetSession(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "session is invalid or expired, please log in again")
			return
		}
		ctx := context.WithValue(r.Context(), sessionCtxKey, sess)
		next(w, r.WithContext(ctx))
	}
}

// requireRole further restricts a route to specific roles (e.g. admin-only).
// Must be used after requireAuth has populated the session in context.
func (s *Server) requireRole(roles ...store.Role) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			sess := sessionFromContext(r)
			for _, role := range roles {
				if sess.Role == role {
					next(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "you don't have permission to do that")
		}
	}
}

func sessionFromContext(r *http.Request) *store.Session {
	sess, _ := r.Context().Value(sessionCtxKey).(*store.Session)
	return sess
}
