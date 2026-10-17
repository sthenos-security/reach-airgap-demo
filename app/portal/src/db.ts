import Database from "better-sqlite3";
import fs from "node:fs";
import path from "node:path";

const rootDir = path.resolve(__dirname, "..", "..");
const dbPath = process.env.TREASURY_DB_PATH ?? path.join(rootDir, "data", "treasury.db");
const schemaPath = path.join(rootDir, "scripts", "schema.sql");

fs.mkdirSync(path.dirname(dbPath), { recursive: true });

export const db = new Database(dbPath);
db.pragma("foreign_keys = ON");
db.exec(fs.readFileSync(schemaPath, "utf8"));

export type AuthUser = {
  id: string;
  organization_id: string;
  team_id: string | null;
  email: string;
  name: string;
  role: "admin" | "treasury_manager" | "operator" | "viewer";
  organization_name: string;
  team_name: string | null;
};

export function getUser(userId: string): AuthUser | undefined {
  return db
    .prepare(
      `SELECT u.id, u.organization_id, u.team_id, u.email, u.name, u.role,
              o.name AS organization_name, t.name AS team_name
         FROM users u
         JOIN organizations o ON o.id = u.organization_id
         LEFT JOIN teams t ON t.id = u.team_id
        WHERE u.id = ? AND u.active = 1`
    )
    .get(userId) as AuthUser | undefined;
}

export function audit(input: {
  id: string;
  organizationId?: string;
  actorUserId?: string;
  action: string;
  method: string;
  path: string;
  statusCode: number;
  metadata?: Record<string, unknown>;
}) {
  db.prepare(
    `INSERT INTO audit_records
      (id, organization_id, actor_user_id, service, action, method, path, status_code, metadata)
     VALUES (?, ?, ?, 'portal', ?, ?, ?, ?, ?)`
  ).run(
    input.id,
    input.organizationId ?? null,
    input.actorUserId ?? null,
    input.action,
    input.method,
    input.path,
    input.statusCode,
    JSON.stringify(input.metadata ?? {})
  );
}

