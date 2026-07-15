package api

import (
	"errors"
	"net/http"
	"strings"

	"coldstorage/internal/store"
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

// PATCH /api/lots/{id}
// Edits an existing lot — e.g. reducing (or zeroing out) the quantity
// when a farmer takes some or all of their goods back. The lot's
// previous and new values are written to the lot_logs table
// automatically (see store.UpdateLot), so nothing about the change
// is lost even though the lot row itself is updated in place.
type updateLotRequest struct {
	ItemName     string  `json:"item_name"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
	RackNo       string  `json:"rack_no"`
	QualityGrade string  `json:"quality_grade"`
	Note         string  `json:"note"`
}

func (s *Server) handleUpdateLot(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	lotID := r.PathValue("id")

	var req updateLotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ItemName = strings.TrimSpace(req.ItemName)
	req.RackNo = strings.TrimSpace(req.RackNo)
	req.QualityGrade = strings.TrimSpace(req.QualityGrade)
	req.Note = strings.TrimSpace(req.Note)
	if req.Unit == "" {
		req.Unit = "kg"
	}

	if req.ItemName == "" {
		writeError(w, http.StatusBadRequest, "item_name is required")
		return
	}
	// Unlike adding a lot, 0 is allowed here — it's how you record a
	// farmer taking everything in a lot back.
	if req.Quantity < 0 {
		writeError(w, http.StatusBadRequest, "quantity can't be negative")
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

	updated, err := s.db.UpdateLot(r.Context(), sess.CompanyID, lotID, req.ItemName, req.Quantity, req.Unit, req.RackNo, req.QualityGrade, req.Note, sess.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "lot not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not update lot")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// GET /api/logs
// Lists every lot edit under the caller's company — the "Logs" tab,
// e.g. a record of farmers taking goods back.
func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	logs, err := s.db.ListLotLogs(r.Context(), sess.CompanyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load logs")
		return
	}
	writeJSON(w, http.StatusOK, logs)
}
