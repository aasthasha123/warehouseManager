package api

import (
	"errors"
	"net/http"
	"strings"

	"coldstorage/internal/store"
)

// POST /api/farmers
// A Supervisor (or Admin) onboards a new farmer. The response
// includes qr_token — the frontend encodes this into the QR code
// printed/shown to the farmer. We deliberately don't put the Aadhar
// number itself in the QR so a lost/photographed card doesn't leak it.
type createFarmerRequest struct {
	Aadhar string `json:"aadhar"`
	Name   string `json:"name"`
	Place  string `json:"place"`
	Phone  string `json:"phone"`
}

func (s *Server) handleCreateFarmer(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)

	var req createFarmerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	aadhar := cleanAadhar(req.Aadhar)
	name := strings.TrimSpace(req.Name)
	place := strings.TrimSpace(req.Place)
	phone := cleanPhone(req.Phone)

	if !isValidAadhar(aadhar) {
		writeError(w, http.StatusBadRequest, "aadhar must be exactly 12 digits")
		return
	}
	if name == "" || place == "" {
		writeError(w, http.StatusBadRequest, "name and place are required")
		return
	}
	if !isValidPhone(phone) {
		writeError(w, http.StatusBadRequest, "phone must be a valid 10-digit mobile number")
		return
	}

	farmer, err := s.db.CreateFarmer(r.Context(), sess.CompanyID, aadhar, name, place, phone, sess.UserID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrAadharExists):
			writeError(w, http.StatusConflict, "a farmer with this Aadhar number is already registered")
		case errors.Is(err, store.ErrFreeTierLimit):
			writeError(w, http.StatusPaymentRequired, "free tier limit reached — upgrade your plan to onboard more farmers")
		default:
			writeError(w, http.StatusInternalServerError, "could not register farmer")
		}
		return
	}

	writeJSON(w, http.StatusCreated, farmer)
}

// GET /api/farmers
// Lists every farmer onboarded under the caller's company.
func (s *Server) handleListFarmers(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	farmers, err := s.db.ListFarmersByCompany(r.Context(), sess.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load farmers")
		return
	}
	writeJSON(w, http.StatusOK, farmers)
}

// GET /api/farmers/{id}
func (s *Server) handleGetFarmer(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	id := r.PathValue("id")

	farmer, err := s.db.GetFarmer(r.Context(), id)
	if err != nil || farmer.CompanyID != sess.CompanyID {
		writeError(w, http.StatusNotFound, "farmer not found")
		return
	}
	writeJSON(w, http.StatusOK, farmer)
}

// GET /api/farmers/scan/{token}
// This is what the frontend calls right after a Supervisor scans a
// farmer's QR code — it resolves the opaque token back to the full
// farmer record (including lot history) so a new lot can be added.
func (s *Server) handleScanFarmer(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	token := r.PathValue("token")

	farmer, err := s.db.GetFarmerByToken(r.Context(), token)
	if err != nil || farmer.CompanyID != sess.CompanyID {
		writeError(w, http.StatusNotFound, "this QR code doesn't match a farmer in your company")
		return
	}
	writeJSON(w, http.StatusOK, farmer)
}
