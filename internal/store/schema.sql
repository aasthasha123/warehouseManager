-- Schema for the cold storage platform. Applied automatically on
-- startup by DB.migrate() (see store.go) — every statement is
-- idempotent (IF NOT EXISTS), so it's safe to run on every boot.

CREATE TABLE IF NOT EXISTS companies (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    plan       TEXT NOT NULL DEFAULT 'free',
    owner_id   TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    company_id    TEXT NOT NULL REFERENCES companies(id),
    name          TEXT NOT NULL,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL, -- 'admin' | 'supervisor'
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_users_company ON users(company_id);

CREATE TABLE IF NOT EXISTS farmers (
    id            TEXT PRIMARY KEY,
    company_id    TEXT NOT NULL REFERENCES companies(id),
    aadhar        TEXT NOT NULL,
    name          TEXT NOT NULL,
    place         TEXT NOT NULL,
    phone         TEXT NOT NULL DEFAULT '',
    qr_token      TEXT NOT NULL UNIQUE,
    registered_by TEXT NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, aadhar)
);
-- ADD COLUMN IF NOT EXISTS covers deployments created before the phone
-- column existed; harmless no-op on a fresh database.
ALTER TABLE farmers ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_farmers_company ON farmers(company_id);
CREATE INDEX IF NOT EXISTS idx_farmers_qr_token ON farmers(qr_token);

CREATE TABLE IF NOT EXISTS lots (
    id            TEXT PRIMARY KEY,
    farmer_id     TEXT NOT NULL REFERENCES farmers(id),
    item_name     TEXT NOT NULL,
    quantity      DOUBLE PRECISION NOT NULL,
    unit          TEXT NOT NULL,
    rack_no       TEXT NOT NULL,
    quality_grade TEXT NOT NULL,
    added_by      TEXT NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_lots_farmer ON lots(farmer_id);

-- Every edit to a lot (e.g. a farmer taking some or all of their
-- goods back) writes one row here with the before/after values, so
-- lots stay editable without losing the history of what changed.
CREATE TABLE IF NOT EXISTS lot_logs (
    id                 TEXT PRIMARY KEY,
    lot_id             TEXT NOT NULL REFERENCES lots(id),
    farmer_id          TEXT NOT NULL REFERENCES farmers(id),
    company_id         TEXT NOT NULL REFERENCES companies(id),
    note               TEXT NOT NULL DEFAULT '',
    prev_item_name     TEXT NOT NULL,
    prev_quantity      DOUBLE PRECISION NOT NULL,
    prev_unit          TEXT NOT NULL,
    prev_rack_no       TEXT NOT NULL,
    prev_quality_grade TEXT NOT NULL,
    new_item_name      TEXT NOT NULL,
    new_quantity       DOUBLE PRECISION NOT NULL,
    new_unit           TEXT NOT NULL,
    new_rack_no        TEXT NOT NULL,
    new_quality_grade  TEXT NOT NULL,
    edited_by          TEXT NOT NULL REFERENCES users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_lot_logs_company ON lot_logs(company_id);
CREATE INDEX IF NOT EXISTS idx_lot_logs_farmer ON lot_logs(farmer_id);
CREATE INDEX IF NOT EXISTS idx_lot_logs_lot ON lot_logs(lot_id);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    company_id TEXT NOT NULL,
    role       TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
