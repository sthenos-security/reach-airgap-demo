import type { NextFunction, Request, Response } from "express";
import { randomUUID } from "node:crypto";
import { audit, AuthUser, getUser } from "./db";

declare global {
  namespace Express {
    interface Request {
      requestId: string;
      user?: AuthUser;
    }
  }
}

const roleRank = {
  viewer: 1,
  operator: 2,
  treasury_manager: 3,
  admin: 4
};

export function requestContext(req: Request, res: Response, next: NextFunction) {
  req.requestId = randomUUID();
  res.setHeader("X-Request-Id", req.requestId);
  next();
}

export function authenticate(req: Request, res: Response, next: NextFunction) {
  const userId = String(req.header("X-User-Id") ?? "u-alice");
  const user = getUser(userId);
  if (!user) {
    return res.status(401).json({ error: "invalid_user" });
  }
  req.user = user;
  next();
}

export function requireRole(minRole: keyof typeof roleRank) {
  return (req: Request, res: Response, next: NextFunction) => {
    if (!req.user || roleRank[req.user.role] < roleRank[minRole]) {
      return res.status(403).json({ error: "forbidden", requiredRole: minRole });
    }
    next();
  };
}

export function auditLogger(req: Request, res: Response, next: NextFunction) {
  const started = Date.now();
  res.on("finish", () => {
    if (req.path === "/favicon.ico") {
      return;
    }
    try {
      audit({
        id: randomUUID(),
        organizationId: req.user?.organization_id,
        actorUserId: req.user?.id,
        action: routeAction(req),
        method: req.method,
        path: req.originalUrl,
        statusCode: res.statusCode,
        metadata: {
          requestId: req.requestId,
          durationMs: Date.now() - started,
          userAgent: req.header("user-agent")
        }
      });
    } catch (error) {
      console.error("portal audit failed", error);
    }
  });
  next();
}

function routeAction(req: Request) {
  if (req.method === "POST" && req.path.includes("beneficiaries")) {
    return "beneficiary.create";
  }
  if (req.path.includes("dashboard")) {
    return "dashboard.read";
  }
  return "request";
}

