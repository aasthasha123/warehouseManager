package store

import "time"

// Role identifies what a logged-in user is allowed to do.
// Farmers are intentionally NOT a Role here — they never get a
// username/password, only a QR token (see Farmer.QRToken).
type Role string

const (
	RoleAdmin      Role = "admin"      // owner of a cold storage company
	RoleSupervisor Role = "supervisor" // in-charge who onboards farmers & logs lots
)

// Plan is the subscription tier for a Company. Only "free" is enforced
// today (see FreeTierFarmerLimit in store.go) — "pro" exists so the
// upgrade path has somewhere to land later.
type Plan string

const (
	PlanFree Plan = "free"
	PlanPro  Plan = "pro"
)

// Company represents one cold storage business on the platform.
// Every User and Farmer belongs to exactly one Company, and all
// data access is scoped by CompanyID.
type Company struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Plan      Plan      `json:"plan"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
}

// User is an Admin (owner) or Supervisor (in-charge) — anyone who
// logs in with an email + password.
type User struct {
	ID           string    `json:"id"`
	CompanyID    string    `json:"company_id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never serialized in API responses
	Role         Role      `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// Farmer is the customer of the cold storage company. They're
// onboarded once by a Supervisor and identified afterwards purely
// by their QR token — no password. The QR token is opaque (random),
// deliberately not the Aadhar number, so the printed/shared QR
// doesn't leak PII if seen by someone else.
type Farmer struct {
	ID           string    `json:"id"`
	CompanyID    string    `json:"company_id"`
	Aadhar       string    `json:"aadhar"`
	Name         string    `json:"name"`
	Place        string    `json:"place"`
	Phone        string    `json:"phone"`
	QRToken      string    `json:"qr_token"`
	RegisteredBy string    `json:"registered_by"` // User.ID of the supervisor
	CreatedAt    time.Time `json:"created_at"`
	Lots         []Lot     `json:"lots"`
}

type FarmerQRDetails struct {
	ID           string    `json:"id"`
	CompanyID    string    `json:"company_id"`
	Aadhar       *string   `json:"aadhar"` // Handles NULL
	Name         string    `json:"name"`
	Place        *string   `json:"place"` // Handles NULL
	Phone        *string   `json:"phone"` // Handles NULL
	QRToken      string    `json:"qr_token"`
	RegisteredBy *string   `json:"registered_by"` // Handles NULL
	CreatedAt    time.Time `json:"created_at"`
	Lots         []Lot     `json:"lots"`
}
// Lot is one batch of produce a farmer has placed in cold storage.
// Scan-and-add appends a new Lot to the farmer's record. Lots can
// also be edited later (e.g. when a farmer takes some goods back) —
// every edit is captured as a LotLog rather than silently overwriting
// history.
type Lot struct {
	ID           string    `json:"id"`
	ItemName     string    `json:"item_name"`
	Quantity     float64   `json:"quantity"`
	Unit         string    `json:"unit"`
	RackNo       string    `json:"rack_no"`
	QualityGrade string    `json:"quality_grade"`
	AddedBy      string    `json:"added_by"` // User.ID of the supervisor who logged it
	CreatedAt    time.Time `json:"created_at"`
}

// Session is an issued bearer token for a logged-in User.
type Session struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	CompanyID string    `json:"company_id"`
	Role      Role      `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// LotLog is one edit made to a Lot — e.g. a supervisor reducing the
// quantity because the farmer took some goods back. Every edit to a
// lot writes one of these with the before/after values, so lots
// stay editable without losing the history of what changed.
// FarmerName is only populated when listing logs (a convenience
// join) — it isn't a stored column.
type LotLog struct {
	ID               string    `json:"id"`
	LotID            string    `json:"lot_id"`
	FarmerID         string    `json:"farmer_id"`
	FarmerName       string    `json:"farmer_name,omitempty"`
	CompanyID        string    `json:"company_id"`
	Note             string    `json:"note"`
	PrevItemName     string    `json:"prev_item_name"`
	PrevQuantity     float64   `json:"prev_quantity"`
	PrevUnit         string    `json:"prev_unit"`
	PrevRackNo       string    `json:"prev_rack_no"`
	PrevQualityGrade string    `json:"prev_quality_grade"`
	NewItemName      string    `json:"new_item_name"`
	NewQuantity      float64   `json:"new_quantity"`
	NewUnit          string    `json:"new_unit"`
	NewRackNo        string    `json:"new_rack_no"`
	NewQualityGrade  string    `json:"new_quality_grade"`
	EditedBy         string    `json:"edited_by"`
	CreatedAt        time.Time `json:"created_at"`
}
