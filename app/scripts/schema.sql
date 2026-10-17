PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS organizations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  plan TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS teams (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id),
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id),
  team_id TEXT REFERENCES teams(id),
  email TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('admin', 'treasury_manager', 'operator', 'viewer')),
  active INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS accounts (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id),
  name TEXT NOT NULL,
  bank_name TEXT NOT NULL,
  currency TEXT NOT NULL,
  balance_cents INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'open',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS beneficiaries (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id),
  name TEXT NOT NULL,
  bank_name TEXT NOT NULL,
  routing_number TEXT NOT NULL,
  account_number_last4 TEXT NOT NULL,
  currency TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transfers (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id),
  source_account_id TEXT NOT NULL REFERENCES accounts(id),
  beneficiary_id TEXT NOT NULL REFERENCES beneficiaries(id),
  amount_cents INTEGER NOT NULL,
  currency TEXT NOT NULL,
  memo TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  requested_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  settled_at TEXT
);

CREATE TABLE IF NOT EXISTS settlement_jobs (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id),
  transfer_id TEXT NOT NULL REFERENCES transfers(id),
  status TEXT NOT NULL DEFAULT 'queued',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY,
  organization_id TEXT NOT NULL REFERENCES organizations(id),
  uploaded_by TEXT NOT NULL REFERENCES users(id),
  filename TEXT NOT NULL,
  content_type TEXT NOT NULL,
  storage_path TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'uploaded',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  indexed_at TEXT
);

CREATE TABLE IF NOT EXISTS audit_records (
  id TEXT PRIMARY KEY,
  organization_id TEXT,
  actor_user_id TEXT,
  service TEXT NOT NULL,
  action TEXT NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  metadata TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_org ON users(organization_id);
CREATE INDEX IF NOT EXISTS idx_accounts_org ON accounts(organization_id);
CREATE INDEX IF NOT EXISTS idx_beneficiaries_org ON beneficiaries(organization_id);
CREATE INDEX IF NOT EXISTS idx_transfers_org_status ON transfers(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_audit_org_created ON audit_records(organization_id, created_at);
CREATE INDEX IF NOT EXISTS idx_documents_org ON documents(organization_id);
CREATE INDEX IF NOT EXISTS idx_settlement_jobs_status ON settlement_jobs(status, updated_at);

INSERT OR IGNORE INTO organizations (id, name, plan) VALUES
  ('org-northstar', 'Northstar Robotics Treasury', 'enterprise'),
  ('org-harbor', 'Harbor Foods Group', 'business');

INSERT OR IGNORE INTO teams (id, organization_id, name) VALUES
  ('team-treasury', 'org-northstar', 'Treasury Operations'),
  ('team-finance', 'org-northstar', 'Finance Control'),
  ('team-harbor-ops', 'org-harbor', 'Payments');

INSERT OR IGNORE INTO users (id, organization_id, team_id, email, name, role) VALUES
  ('u-alice', 'org-northstar', 'team-treasury', 'alice@northstar.example', 'Alice Admin', 'admin'),
  ('u-ben', 'org-northstar', 'team-treasury', 'ben@northstar.example', 'Ben Operator', 'operator'),
  ('u-chen', 'org-northstar', 'team-finance', 'chen@northstar.example', 'Chen Viewer', 'viewer'),
  ('u-harper', 'org-harbor', 'team-harbor-ops', 'harper@harbor.example', 'Harper Manager', 'treasury_manager');

INSERT OR IGNORE INTO accounts (id, organization_id, name, bank_name, currency, balance_cents, status) VALUES
  ('acct-operating', 'org-northstar', 'Operating Account', 'First Ledger Bank', 'USD', 82500000, 'open'),
  ('acct-payroll', 'org-northstar', 'Payroll Account', 'First Ledger Bank', 'USD', 21000000, 'open'),
  ('acct-harbor-main', 'org-harbor', 'Main Disbursement', 'Continental Trust', 'USD', 44250000, 'open');

INSERT OR IGNORE INTO beneficiaries (id, organization_id, name, bank_name, routing_number, account_number_last4, currency, status) VALUES
  ('ben-atlas', 'org-northstar', 'Atlas Components', 'Metro Commercial', '021000021', '8842', 'USD', 'active'),
  ('ben-riverline', 'org-northstar', 'Riverline Logistics', 'Metro Commercial', '121000248', '1931', 'USD', 'active'),
  ('ben-coldchain', 'org-harbor', 'Coldchain Warehousing', 'Union Bank', '026009593', '5520', 'USD', 'active');

