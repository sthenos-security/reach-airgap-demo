import express from "express";
import morgan from "morgan";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { db } from "./db";
import { auditLogger, authenticate, requestContext, requireRole } from "./middleware";

const app = express();
const port = Number(process.env.PORT ?? 3000);
const startedAt = Date.now();

app.use(requestContext);
app.use(morgan("tiny"));
app.use(express.json({ limit: "1mb" }));
app.use(express.static(path.join(__dirname, "..", "public")));

app.get("/health", (_req, res) => {
  const row = db.prepare("SELECT COUNT(*) AS count FROM organizations").get() as { count: number };
  res.json({ ok: true, service: "portal", organizations: row.count, uptimeSeconds: Math.round((Date.now() - startedAt) / 1000) });
});

app.get("/metrics", (_req, res) => {
  const totals = db
    .prepare(
      `SELECT
         (SELECT COUNT(*) FROM users) AS users,
         (SELECT COUNT(*) FROM transfers WHERE status = 'pending') AS pending_transfers,
         (SELECT COUNT(*) FROM audit_records WHERE service = 'portal') AS portal_audits`
    )
    .get() as Record<string, number>;
  res.type("text/plain").send(
    [
      "# HELP treasury_portal_users Total seeded users",
      "# TYPE treasury_portal_users gauge",
      `treasury_portal_users ${totals.users}`,
      "# HELP treasury_portal_pending_transfers Pending transfers",
      "# TYPE treasury_portal_pending_transfers gauge",
      `treasury_portal_pending_transfers ${totals.pending_transfers}`,
      "# HELP treasury_portal_audit_records Portal audit records",
      "# TYPE treasury_portal_audit_records counter",
      `treasury_portal_audit_records ${totals.portal_audits}`
    ].join("\n")
  );
});

app.use("/api", authenticate, auditLogger);

app.get("/api/me", (req, res) => {
  res.json({ user: req.user });
});

app.get("/api/dashboard", requireRole("viewer"), (req, res) => {
  const orgId = req.user!.organization_id;
  const organization = db.prepare("SELECT id, name, plan, created_at FROM organizations WHERE id = ?").get(orgId);
  const users = db
    .prepare(
      `SELECT u.id, u.name, u.email, u.role, t.name AS team_name
         FROM users u LEFT JOIN teams t ON t.id = u.team_id
        WHERE u.organization_id = ?
        ORDER BY u.created_at DESC`
    )
    .all(orgId);
  const accounts = db
    .prepare(
      `SELECT id, name, bank_name, currency, balance_cents, status
         FROM accounts
        WHERE organization_id = ?
        ORDER BY name`
    )
    .all(orgId);
  const beneficiaries = db
    .prepare(
      `SELECT id, name, bank_name, account_number_last4, currency, status, created_at
         FROM beneficiaries
        WHERE organization_id = ?
        ORDER BY created_at DESC`
    )
    .all(orgId);
  const transfers = db
    .prepare(
      `SELECT tr.id, tr.amount_cents, tr.currency, tr.status, tr.memo, tr.created_at, tr.settled_at,
              a.name AS source_account, b.name AS beneficiary, u.name AS requested_by
         FROM transfers tr
         JOIN accounts a ON a.id = tr.source_account_id
         JOIN beneficiaries b ON b.id = tr.beneficiary_id
         JOIN users u ON u.id = tr.requested_by
        WHERE tr.organization_id = ?
        ORDER BY tr.created_at DESC
        LIMIT 12`
    )
    .all(orgId);
  const auditRecords = db
    .prepare(
      `SELECT service, action, method, path, status_code, created_at
         FROM audit_records
        WHERE organization_id = ?
        ORDER BY created_at DESC
        LIMIT 10`
    )
    .all(orgId);

  res.json({ organization, users, accounts, beneficiaries, transfers, auditRecords });
});

app.post("/api/beneficiaries", requireRole("treasury_manager"), (req, res) => {
  const { name, bankName, routingNumber, accountNumber, currency } = req.body ?? {};
  if (!name || !bankName || !routingNumber || !accountNumber || !currency) {
    return res.status(400).json({ error: "missing_required_fields" });
  }
  const id = `ben-${randomUUID()}`;
  const last4 = String(accountNumber).slice(-4);
  db.prepare(
    `INSERT INTO beneficiaries
      (id, organization_id, name, bank_name, routing_number, account_number_last4, currency, status)
     VALUES (?, ?, ?, ?, ?, ?, ?, 'active')`
  ).run(id, req.user!.organization_id, name, bankName, routingNumber, last4, currency);
  res.status(201).json({ id, name, bankName, currency, status: "active" });
});

app.get("/api/audit", requireRole("admin"), (req, res) => {
  const rows = db
    .prepare(
      `SELECT ar.id, ar.service, ar.action, ar.method, ar.path, ar.status_code, ar.created_at,
              u.name AS actor
         FROM audit_records ar
         LEFT JOIN users u ON u.id = ar.actor_user_id
        WHERE ar.organization_id = ?
        ORDER BY ar.created_at DESC
        LIMIT 50`
    )
    .all(req.user!.organization_id);
  res.json({ auditRecords: rows });
});

app.listen(port, () => {
  console.log(`treasury portal listening on http://localhost:${port}`);
});

