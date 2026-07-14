// Package store is the persistence layer, backed by Postgres
// (e.g. Render Postgres) via database/sql and github.com/lib/pq.
//
// Every read/write goes through a method on *DB — handlers never
// write SQL directly — so the schema and query details stay in one
// place (this file + schema.sql).
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrEmailTaken    = errors.New("email already registered")
	ErrAadharExists  = errors.New("farmer with this aadhar already registered for this company")
	ErrFreeTierLimit = errors.New("free tier farmer limit reached")
)

// FreeTierFarmerLimit is how many farmers a "free" plan company may
// onboard before they need to upgrade. Deliberately a plain const
// for now rather than config — easy to find, easy to change.
const FreeTierFarmerLimit = 25

// SessionTTL is how long a login token stays valid.
const SessionTTL = 7 * 24 * time.Hour

//go:embed schema.sql
var schemaSQL string

type DB struct {
	pool *sql.DB
}

// Open connects to Postgres using dsn (e.g. the DATABASE_URL Render
// gives you) and applies the schema. The schema is idempotent
// (CREATE TABLE IF NOT EXISTS), so this is safe to call on every boot.
func Open(ctx context.Context, dsn string) (*DB, error) {
	pool, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	db := &DB{pool: pool}
	if err := db.migrate(ctx); err != nil {
		return nil, fmt.Errorf("running schema migration: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.pool.Close()
}

func (db *DB) migrate(ctx context.Context) error {
	_, err := db.pool.ExecContext(ctx, schemaSQL)
	return err
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func newToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isUniqueViolation reports whether err is a Postgres unique
// constraint violation, optionally matching a specific constraint name.
func isUniqueViolation(err error, constraint string) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	if pqErr.Code != "23505" { // unique_violation
		return false
	}
	return constraint == "" || pqErr.Constraint == constraint
}

// ---------- Companies & Users ----------

// CreateCompanyWithOwner creates a Company and its first Admin user
// together, since a signup always produces both. Runs in a single
// transaction so a failure partway through never leaves an orphaned
// company with no owner.
func (db *DB) CreateCompanyWithOwner(ctx context.Context, companyName, ownerName, email, passwordHash string) (*Company, *User, error) {
	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	company := &Company{
		ID:        newID("co"),
		Name:      companyName,
		Plan:      PlanFree,
		CreatedAt: time.Now(),
	}
	owner := &User{
		ID:           newID("usr"),
		CompanyID:    company.ID,
		Name:         ownerName,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         RoleAdmin,
		CreatedAt:    time.Now(),
	}
	company.OwnerID = owner.ID

	_, err = tx.ExecContext(ctx,
		`INSERT INTO companies (id, name, plan, owner_id, created_at) VALUES ($1,$2,$3,$4,$5)`,
		company.ID, company.Name, company.Plan, company.OwnerID, company.CreatedAt)
	if err != nil {
		return nil, nil, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, company_id, name, email, password_hash, role, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		owner.ID, owner.CompanyID, owner.Name, owner.Email, owner.PasswordHash, owner.Role, owner.CreatedAt)
	if err != nil {
		if isUniqueViolation(err, "") {
			return nil, nil, ErrEmailTaken
		}
		return nil, nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return company, owner, nil
}

// CreateSupervisor adds a Supervisor user under an existing company.
func (db *DB) CreateSupervisor(ctx context.Context, companyID, name, email, passwordHash string) (*User, error) {
	u := &User{
		ID:           newID("usr"),
		CompanyID:    companyID,
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         RoleSupervisor,
		CreatedAt:    time.Now(),
	}
	_, err := db.pool.ExecContext(ctx,
		`INSERT INTO users (id, company_id, name, email, password_hash, role, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		u.ID, u.CompanyID, u.Name, u.Email, u.PasswordHash, u.Role, u.CreatedAt)
	if err != nil {
		if isUniqueViolation(err, "") {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return u, nil
}

func (db *DB) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := db.pool.QueryRowContext(ctx,
		`SELECT id, company_id, name, email, password_hash, role, created_at FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.CompanyID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (db *DB) GetUser(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := db.pool.QueryRowContext(ctx,
		`SELECT id, company_id, name, email, password_hash, role, created_at FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.CompanyID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ListSupervisorsByCompany returns every Supervisor (not the Admin)
// under a company, for a "Team" view.
func (db *DB) ListSupervisorsByCompany(ctx context.Context, companyID string) ([]*User, error) {
	rows, err := db.pool.QueryContext(ctx,
		`SELECT id, company_id, name, email, password_hash, role, created_at
		 FROM users WHERE company_id = $1 AND role = $2 ORDER BY created_at DESC`,
		companyID, RoleSupervisor,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*User{}
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.CompanyID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (db *DB) GetCompany(ctx context.Context, id string) (*Company, error) {
	c := &Company{}
	err := db.pool.QueryRowContext(ctx,
		`SELECT id, name, plan, owner_id, created_at FROM companies WHERE id = $1`,
		id,
	).Scan(&c.ID, &c.Name, &c.Plan, &c.OwnerID, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// ---------- Sessions ----------

func (db *DB) CreateSession(ctx context.Context, userID, companyID string, role Role) (*Session, error) {
	s := &Session{
		Token:     newToken(),
		UserID:    userID,
		CompanyID: companyID,
		Role:      role,
		ExpiresAt: time.Now().Add(SessionTTL),
	}
	_, err := db.pool.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, company_id, role, expires_at) VALUES ($1,$2,$3,$4,$5)`,
		s.Token, s.UserID, s.CompanyID, s.Role, s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (db *DB) GetSession(ctx context.Context, token string) (*Session, error) {
	s := &Session{}
	err := db.pool.QueryRowContext(ctx,
		`SELECT token, user_id, company_id, role, expires_at FROM sessions WHERE token = $1 AND expires_at > now()`,
		token,
	).Scan(&s.Token, &s.UserID, &s.CompanyID, &s.Role, &s.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (db *DB) DeleteSession(ctx context.Context, token string) error {
	_, err := db.pool.ExecContext(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// ---------- Farmers & Lots ----------

// CreateFarmer onboards a new farmer under a company, enforcing the
// free-tier limit and one-Aadhar-per-company uniqueness. The plan
// check + count + insert run inside a transaction that locks the
// company row (SELECT ... FOR UPDATE), so two simultaneous
// registrations can't both slip in past the free-tier limit.
func (db *DB) CreateFarmer(ctx context.Context, companyID, aadhar, name, place, phone, registeredBy string) (*Farmer, error) {
	tx, err := db.pool.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var plan Plan
	err = tx.QueryRowContext(ctx, `SELECT plan FROM companies WHERE id = $1 FOR UPDATE`, companyID).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if plan == PlanFree {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM farmers WHERE company_id = $1`, companyID).Scan(&count); err != nil {
			return nil, err
		}
		if count >= FreeTierFarmerLimit {
			return nil, ErrFreeTierLimit
		}
	}

	f := &Farmer{
		ID:           newID("frm"),
		CompanyID:    companyID,
		Aadhar:       aadhar,
		Name:         name,
		Place:        place,
		Phone:        phone,
		QRToken:      newToken(),
		RegisteredBy: registeredBy,
		CreatedAt:    time.Now(),
		Lots:         []Lot{},
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO farmers (id, company_id, aadhar, name, place, phone, qr_token, registered_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		f.ID, f.CompanyID, f.Aadhar, f.Name, f.Place, f.Phone, f.QRToken, f.RegisteredBy, f.CreatedAt)
	if err != nil {
		if isUniqueViolation(err, "farmers_company_id_aadhar_key") {
			return nil, ErrAadharExists
		}
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return f, nil
}

func (db *DB) GetFarmer(ctx context.Context, id string) (*Farmer, error) {
	f := &Farmer{}
	err := db.pool.QueryRowContext(ctx,
		`SELECT id, company_id, aadhar, name, place, phone, qr_token, registered_by, created_at
		 FROM farmers WHERE id = $1`,
		id,
	).Scan(&f.ID, &f.CompanyID, &f.Aadhar, &f.Name, &f.Place, &f.Phone, &f.QRToken, &f.RegisteredBy, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	lots, err := db.lotsForFarmer(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	f.Lots = lots
	return f, nil
}

func (db *DB) GetFarmerByToken(ctx context.Context, token string) (*Farmer, error) {
	f := &Farmer{}
	err := db.pool.QueryRowContext(ctx,
		`SELECT id, company_id, aadhar, name, place, phone, qr_token, registered_by, created_at
		 FROM farmers WHERE qr_token = $1`,
		token,
	).Scan(&f.ID, &f.CompanyID, &f.Aadhar, &f.Name, &f.Place, &f.Phone, &f.QRToken, &f.RegisteredBy, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	lots, err := db.lotsForFarmer(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	f.Lots = lots
	return f, nil
}

// ListFarmersByCompany returns every farmer under a company with
// their lots attached, using one query for the farmers and one for
// all their lots (rather than N+1 queries per farmer).
func (db *DB) ListFarmersByCompany(ctx context.Context, companyID string) ([]*Farmer, error) {
	rows, err := db.pool.QueryContext(ctx,
		`SELECT id, company_id, aadhar, name, place, phone, qr_token, registered_by, created_at
		 FROM farmers WHERE company_id = $1 ORDER BY created_at DESC`,
		companyID,
	)
	if err != nil {
		return nil, err
	}
	farmers := []*Farmer{}
	byID := map[string]*Farmer{}
	ids := []string{}
	for rows.Next() {
		f := &Farmer{}
		if err := rows.Scan(&f.ID, &f.CompanyID, &f.Aadhar, &f.Name, &f.Place, &f.Phone, &f.QRToken, &f.RegisteredBy, &f.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		f.Lots = []Lot{}
		farmers = append(farmers, f)
		byID[f.ID] = f
		ids = append(ids, f.ID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if len(ids) == 0 {
		return farmers, nil
	}

	lotRows, err := db.pool.QueryContext(ctx,
		`SELECT id, farmer_id, item_name, quantity, unit, rack_no, quality_grade, added_by, created_at
		 FROM lots WHERE farmer_id = ANY($1) ORDER BY created_at DESC`,
		pq.Array(ids),
	)
	if err != nil {
		return nil, err
	}
	defer lotRows.Close()
	for lotRows.Next() {
		var farmerID string
		l := Lot{}
		if err := lotRows.Scan(&l.ID, &farmerID, &l.ItemName, &l.Quantity, &l.Unit, &l.RackNo, &l.QualityGrade, &l.AddedBy, &l.CreatedAt); err != nil {
			return nil, err
		}
		if f, ok := byID[farmerID]; ok {
			f.Lots = append(f.Lots, l)
		}
	}
	return farmers, lotRows.Err()
}

func (db *DB) lotsForFarmer(ctx context.Context, farmerID string) ([]Lot, error) {
	rows, err := db.pool.QueryContext(ctx,
		`SELECT id, farmer_id, item_name, quantity, unit, rack_no, quality_grade, added_by, created_at
		 FROM lots WHERE farmer_id = $1 ORDER BY created_at DESC`,
		farmerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lots := []Lot{}
	for rows.Next() {
		var fID string
		l := Lot{}
		if err := rows.Scan(&l.ID, &fID, &l.ItemName, &l.Quantity, &l.Unit, &l.RackNo, &l.QualityGrade, &l.AddedBy, &l.CreatedAt); err != nil {
			return nil, err
		}
		lots = append(lots, l)
	}
	return lots, rows.Err()
}

// AddLot appends a new lot to a farmer's record and returns the
// farmer with the full, updated lot history. Every scan-and-add call
// goes through here — lots are only ever inserted, never edited or
// removed, so this is a full history of what that farmer has stored.
func (db *DB) AddLot(ctx context.Context, farmerID, itemName string, quantity float64, unit, rackNo, qualityGrade, addedBy string) (*Farmer, error) {
	lot := Lot{
		ID:           newID("lot"),
		ItemName:     itemName,
		Quantity:     quantity,
		Unit:         unit,
		RackNo:       rackNo,
		QualityGrade: qualityGrade,
		AddedBy:      addedBy,
		CreatedAt:    time.Now(),
	}
	_, err := db.pool.ExecContext(ctx,
		`INSERT INTO lots (id, farmer_id, item_name, quantity, unit, rack_no, quality_grade, added_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		lot.ID, farmerID, lot.ItemName, lot.Quantity, lot.Unit, lot.RackNo, lot.QualityGrade, lot.AddedBy, lot.CreatedAt)
	if err != nil {
		return nil, err
	}
	return db.GetFarmer(ctx, farmerID)
}
