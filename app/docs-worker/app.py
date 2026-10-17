from __future__ import annotations

import json
import os
import re
import sqlite3
import threading
import time
import uuid
from pathlib import Path
from typing import Callable, Iterable
from urllib.parse import unquote
from wsgiref.simple_server import make_server


ROOT = Path(__file__).resolve().parents[1]
DB_PATH = Path(os.environ.get("TREASURY_DB_PATH", ROOT / "data" / "treasury.db"))
SCHEMA_PATH = ROOT / "scripts" / "schema.sql"
UPLOAD_ROOT = Path(os.environ.get("TREASURY_UPLOAD_ROOT", ROOT / "uploads" / "docs"))
PORT = int(os.environ.get("PORT", "8090"))

StartResponse = Callable[[str, list[tuple[str, str]]], None]
WSGIApp = Callable[[dict, StartResponse], Iterable[bytes]]

ROLE_RANK = {
    "viewer": 1,
    "operator": 2,
    "treasury_manager": 3,
    "admin": 4,
}


def connect() -> sqlite3.Connection:
    DB_PATH.parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys = ON")
    return conn


def init_db() -> None:
    with connect() as conn:
        conn.executescript(SCHEMA_PATH.read_text())
    UPLOAD_ROOT.mkdir(parents=True, exist_ok=True)


def lookup_user(user_id: str) -> dict | None:
    with connect() as conn:
        row = conn.execute(
            """
            SELECT id, organization_id, name, email, role
              FROM users
             WHERE id = ? AND active = 1
            """,
            (user_id,),
        ).fetchone()
    return dict(row) if row else None


class RequestContextMiddleware:
    def __init__(self, app: WSGIApp):
        self.app = app

    def __call__(self, environ: dict, start_response: StartResponse) -> Iterable[bytes]:
        environ["treasury.request_id"] = f"req-{uuid.uuid4()}"
        return self.app(environ, start_response)


class CorsMiddleware:
    def __init__(self, app: WSGIApp):
        self.app = app

    def __call__(self, environ: dict, start_response: StartResponse) -> Iterable[bytes]:
        if environ["REQUEST_METHOD"] == "OPTIONS":
            start_response(
                "204 No Content",
                [
                    ("Access-Control-Allow-Origin", "*"),
                    ("Access-Control-Allow-Headers", "Content-Type, X-User-Id, X-File-Name"),
                    ("Access-Control-Allow-Methods", "GET, POST, OPTIONS"),
                    ("X-Request-Id", environ.get("treasury.request_id", "")),
                ],
            )
            return [b""]
        return self.app(environ, start_response)


class AuditMiddleware:
    def __init__(self, app: WSGIApp):
        self.app = app

    def __call__(self, environ: dict, start_response: StartResponse) -> Iterable[bytes]:
        status_holder = {"code": 500}

        def capture(status: str, headers: list[tuple[str, str]], exc_info=None):
            status_holder["code"] = int(status.split(" ", 1)[0])
            headers = [
                ("Access-Control-Allow-Origin", "*"),
                ("X-Request-Id", environ.get("treasury.request_id", "")),
                *headers,
            ]
            return start_response(status, headers)

        try:
            return list(self.app(environ, capture))
        finally:
            path = environ.get("PATH_INFO", "")
            if path not in {"/health", "/metrics"}:
                self._write_audit(environ, status_holder["code"])

    def _write_audit(self, environ: dict, status_code: int) -> None:
        user = environ.get("treasury.user") or {}
        try:
            with connect() as conn:
                conn.execute(
                    """
                    INSERT INTO audit_records
                      (id, organization_id, actor_user_id, service, action, method, path, status_code, metadata)
                    VALUES (?, ?, ?, 'docs-worker', ?, ?, ?, ?, ?)
                    """,
                    (
                        f"audit-{uuid.uuid4()}",
                        user.get("organization_id"),
                        user.get("id"),
                        action_for(environ),
                        environ.get("REQUEST_METHOD", ""),
                        environ.get("PATH_INFO", ""),
                        status_code,
                        json.dumps({"request_id": environ.get("treasury.request_id")}),
                    ),
                )
        except Exception as exc:
            print(f"docs-worker audit failed: {exc}")


class AuthMiddleware:
    def __init__(self, app: WSGIApp):
        self.app = app

    def __call__(self, environ: dict, start_response: StartResponse) -> Iterable[bytes]:
        if environ.get("PATH_INFO") in {"/health", "/metrics"}:
            return self.app(environ, start_response)
        user_id = environ.get("HTTP_X_USER_ID") or "u-alice"
        user = lookup_user(user_id)
        if not user:
            return json_response(start_response, 401, {"error": "invalid_user"})
        environ["treasury.user"] = user
        return self.app(environ, start_response)


class Router:
    def __call__(self, environ: dict, start_response: StartResponse) -> Iterable[bytes]:
        method = environ["REQUEST_METHOD"]
        path = environ.get("PATH_INFO", "")
        if method == "GET" and path == "/health":
            return health(start_response)
        if method == "GET" and path == "/metrics":
            return metrics(start_response)
        if method == "GET" and path == "/documents":
            return require_role(environ, start_response, "viewer", list_documents)
        if method == "POST" and path == "/documents":
            return require_role(environ, start_response, "operator", upload_document)
        match = re.fullmatch(r"/documents/([^/]+)/download", path)
        if method == "GET" and match:
            environ["treasury.document_id"] = unquote(match.group(1))
            return require_role(environ, start_response, "viewer", download_document)
        return json_response(start_response, 404, {"error": "not_found"})


def require_role(
    environ: dict,
    start_response: StartResponse,
    min_role: str,
    handler: Callable[[dict, StartResponse], Iterable[bytes]],
) -> Iterable[bytes]:
    user = environ.get("treasury.user") or {}
    if ROLE_RANK.get(user.get("role"), 0) < ROLE_RANK[min_role]:
        return json_response(start_response, 403, {"error": "forbidden", "required_role": min_role})
    return handler(environ, start_response)


def health(start_response: StartResponse) -> Iterable[bytes]:
    with connect() as conn:
        count = conn.execute("SELECT COUNT(*) AS count FROM documents").fetchone()["count"]
    return json_response(start_response, 200, {"ok": True, "service": "docs-worker", "documents": count})


def metrics(start_response: StartResponse) -> Iterable[bytes]:
    with connect() as conn:
        uploaded = conn.execute("SELECT COUNT(*) AS count FROM documents WHERE status = 'uploaded'").fetchone()["count"]
        indexed = conn.execute("SELECT COUNT(*) AS count FROM documents WHERE status = 'indexed'").fetchone()["count"]
    body = "\n".join(
        [
            "# TYPE treasury_docs_uploaded gauge",
            f"treasury_docs_uploaded {uploaded}",
            "# TYPE treasury_docs_indexed counter",
            f"treasury_docs_indexed {indexed}",
            "",
        ]
    )
    return response(start_response, 200, body.encode(), [("Content-Type", "text/plain; charset=utf-8")])


def list_documents(environ: dict, start_response: StartResponse) -> Iterable[bytes]:
    user = environ["treasury.user"]
    with connect() as conn:
        rows = conn.execute(
            """
            SELECT d.id, d.filename, d.content_type, d.size_bytes, d.status, d.created_at, d.indexed_at,
                   u.name AS uploaded_by
              FROM documents d
              JOIN users u ON u.id = d.uploaded_by
             WHERE d.organization_id = ?
             ORDER BY d.created_at DESC
            """,
            (user["organization_id"],),
        ).fetchall()
    return json_response(start_response, 200, {"documents": [dict(row) for row in rows]})


def upload_document(environ: dict, start_response: StartResponse) -> Iterable[bytes]:
    user = environ["treasury.user"]
    filename = safe_filename(environ.get("HTTP_X_FILE_NAME") or "treasury-document.bin")
    content_type = environ.get("CONTENT_TYPE") or "application/octet-stream"
    length = int(environ.get("CONTENT_LENGTH") or "0")
    if length <= 0:
        return json_response(start_response, 400, {"error": "empty_upload"})
    body = environ["wsgi.input"].read(length)
    document_id = f"doc-{uuid.uuid4()}"
    org_dir = UPLOAD_ROOT / user["organization_id"]
    org_dir.mkdir(parents=True, exist_ok=True)
    storage_path = org_dir / f"{document_id}-{filename}"
    storage_path.write_bytes(body)
    with connect() as conn:
        conn.execute(
            """
            INSERT INTO documents
              (id, organization_id, uploaded_by, filename, content_type, storage_path, size_bytes, status)
            VALUES (?, ?, ?, ?, ?, ?, ?, 'uploaded')
            """,
            (
                document_id,
                user["organization_id"],
                user["id"],
                filename,
                content_type,
                str(storage_path),
                len(body),
            ),
        )
    return json_response(
        start_response,
        201,
        {"id": document_id, "filename": filename, "size_bytes": len(body), "status": "uploaded"},
    )


def download_document(environ: dict, start_response: StartResponse) -> Iterable[bytes]:
    user = environ["treasury.user"]
    document_id = environ["treasury.document_id"]
    with connect() as conn:
        row = conn.execute(
            """
            SELECT filename, content_type, storage_path
              FROM documents
             WHERE id = ? AND organization_id = ?
            """,
            (document_id, user["organization_id"]),
        ).fetchone()
    if not row:
        return json_response(start_response, 404, {"error": "document_not_found"})
    storage_path = Path(row["storage_path"])
    if not storage_path.exists():
        return json_response(start_response, 410, {"error": "document_missing_from_storage"})
    headers = [
        ("Content-Type", row["content_type"]),
        ("Content-Disposition", f'attachment; filename="{row["filename"]}"'),
    ]
    return response(start_response, 200, storage_path.read_bytes(), headers)


def index_worker(stop_event: threading.Event) -> None:
    while not stop_event.wait(4):
        try:
            with connect() as conn:
                row = conn.execute(
                    """
                    SELECT id
                      FROM documents
                     WHERE status = 'uploaded'
                     ORDER BY created_at
                     LIMIT 1
                    """
                ).fetchone()
                if not row:
                    continue
                conn.execute(
                    "UPDATE documents SET status = 'indexed', indexed_at = CURRENT_TIMESTAMP WHERE id = ?",
                    (row["id"],),
                )
                conn.execute(
                    """
                    INSERT INTO audit_records
                      (id, organization_id, actor_user_id, service, action, method, path, status_code, metadata)
                    SELECT ?, organization_id, NULL, 'docs-worker', 'document.indexed', 'WORKER', ?, 200, ?
                      FROM documents
                     WHERE id = ?
                    """,
                    (
                        f"audit-{uuid.uuid4()}",
                        f"/documents/{row['id']}",
                        json.dumps({"document_id": row["id"]}),
                        row["id"],
                    ),
                )
        except Exception as exc:
            print(f"docs-worker indexer failed: {exc}")


def action_for(environ: dict) -> str:
    method = environ.get("REQUEST_METHOD", "")
    path = environ.get("PATH_INFO", "")
    if method == "POST" and path == "/documents":
        return "document.upload"
    if method == "GET" and path.endswith("/download"):
        return "document.download"
    if method == "GET" and path == "/documents":
        return "document.list"
    return "request"


def safe_filename(value: str) -> str:
    base = Path(value).name
    return re.sub(r"[^A-Za-z0-9._-]", "_", base)[:120] or "treasury-document.bin"


def json_response(start_response: StartResponse, status: int, payload: dict) -> Iterable[bytes]:
    return response(
        start_response,
        status,
        json.dumps(payload).encode(),
        [("Content-Type", "application/json")],
    )


def response(start_response: StartResponse, status: int, body: bytes, headers: list[tuple[str, str]]) -> Iterable[bytes]:
    status_text = {
        200: "OK",
        201: "Created",
        204: "No Content",
        400: "Bad Request",
        401: "Unauthorized",
        403: "Forbidden",
        404: "Not Found",
        410: "Gone",
        500: "Internal Server Error",
    }.get(status, "OK")
    start_response(f"{status} {status_text}", [("Content-Length", str(len(body))), *headers])
    return [body]


def build_app() -> WSGIApp:
    return RequestContextMiddleware(CorsMiddleware(AuditMiddleware(AuthMiddleware(Router()))))


if __name__ == "__main__":
    init_db()
    stop = threading.Event()
    thread = threading.Thread(target=index_worker, args=(stop,), daemon=True)
    thread.start()
    try:
        with make_server("", PORT, build_app()) as server:
            print(f"docs worker listening on http://localhost:{PORT}")
            server.serve_forever()
    finally:
        stop.set()

