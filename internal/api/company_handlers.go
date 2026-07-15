package api

import (
	"net/http"

	"coldstorage/internal/store"
)

// GET /api/company/me
// Returns the caller's company along with usage against the free
// tier, so a frontend can show "18 / 25 farmers used" style nudges.
func (s *Server) handleCompanyMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)

	company, err := s.db.GetCompany(r.Context(), sess.CompanyID)
	if err != nil {
		writeError(w, http.StatusNotFound, "company not found")
		return
	}

	farmers, err := s.db.ListFarmersByCompany(r.Context(), sess.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load company usage")
		return
	}

	resp := map[string]any{
		"company":      company,
		"farmer_count": len(farmers),
	}
	if company.Plan == store.PlanFree {
		resp["farmer_limit"] = store.FreeTierFarmerLimit
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/me
// Returns the currently logged-in user plus their company and
// free-tier usage in one call — what the frontend loads right after
// login (or on page refresh, using a stored token) to rebuild the
// signed-in view.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)

	user, err := s.db.GetUser(r.Context(), sess.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	company, err := s.db.GetCompany(r.Context(), sess.CompanyID)
	if err != nil {
		writeError(w, http.StatusNotFound, "company not found")
		return
	}
	farmers, err := s.db.ListFarmersByCompany(r.Context(), sess.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load account")
		return
	}

	resp := map[string]any{
		"user":         user,
		"company":      company,
		"farmer_count": len(farmers),
	}
	if company.Plan == store.PlanFree {
		resp["farmer_limit"] = store.FreeTierFarmerLimit
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /api/supervisors  (admin only)
// Lists every Supervisor added under the caller's company — the "Team" view.
func (s *Server) handleListSupervisors(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	supervisors, err := s.db.ListSupervisorsByCompany(r.Context(), sess.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load team")
		return
	}
	writeJSON(w, http.StatusOK, supervisors)
}
