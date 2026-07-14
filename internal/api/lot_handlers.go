package api

import (
	"net/http"
	"strings"
)

// POST /api/farmers/{id}/lots
// Appends a new lot to the farmer's record. Called after a
// Supervisor scans the farmer's QR (or looks them up by ID) —
// nothing is ever overwritten, so this builds a running history.
type addLotRequest struct {
	ItemName     string  `json:"item_name"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
	RackNo       string  `json:"rack_no"`
	QualityGrade string  `json:"quality_grade"`
}

func (s *Server) handleAddLot(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	farmerID := r.PathValue("id")

	farmer, err := s.db.GetFarmer(r.Context(), farmerID)
	if err != nil || farmer.CompanyID != sess.CompanyID {
		writeError(w, http.StatusNotFound, "farmer not found")
		return
	}

	var req addLotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ItemName = strings.TrimSpace(req.ItemName)
	req.RackNo = strings.TrimSpace(req.RackNo)
	req.QualityGrade = strings.TrimSpace(req.QualityGrade)
	if req.Unit == "" {
		req.Unit = "kg"
	}

	if req.ItemName == "" {
		writeError(w, http.StatusBadRequest, "item_name is required")
		return
	}
	if req.Quantity <= 0 {
		writeError(w, http.StatusBadRequest, "quantity must be greater than 0")
		return
	}
	if req.RackNo == "" {
		writeError(w, http.StatusBadRequest, "rack_no is required")
		return
	}
	if req.QualityGrade == "" {
		writeError(w, http.StatusBadRequest, "quality_grade is required")
		return
	}

	updated, err := s.db.AddLot(r.Context(), farmer.ID, req.ItemName, req.Quantity, req.Unit, req.RackNo, req.QualityGrade, sess.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add lot")
		return
	}

	writeJSON(w, http.StatusCreated, updated)
}
