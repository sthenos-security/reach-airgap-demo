package com.sthenos.treasury;

import com.sun.net.httpserver.Headers;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.UUID;

public final class RiskApi {
    private static final String DEFAULT_DB = "../data/treasury.db";
    private static final int DEFAULT_PORT = 8070;
    private final String dbUrl;
    private final long startedAt = System.currentTimeMillis();

    public RiskApi(String dbPath) {
        this.dbUrl = "jdbc:sqlite:" + dbPath;
    }

    public static void main(String[] args) throws Exception {
        String dbPath = System.getenv().getOrDefault("TREASURY_DB_PATH", DEFAULT_DB);
        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", String.valueOf(DEFAULT_PORT)));
        RiskApi app = new RiskApi(dbPath);
        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);
        server.createContext("/health", app::health);
        server.createContext("/metrics", app::metrics);
        server.createContext("/risk/exposure", app.withAuth("viewer", app::riskExposure));
        server.createContext("/risk/limits", app.withAuth("treasury_manager", app::riskLimits));
        server.createContext("/risk/audit", app.withAuth("admin", app::riskAudit));
        server.start();
        System.out.println("treasury risk-api listening on http://localhost:" + port);
    }

    private HttpHandler withAuth(String minRole, AuthedHandler handler) {
        return exchange -> {
            User user = authenticate(exchange);
            if (user == null) {
                respond(exchange, 401, "{\"error\":\"invalid_user\"}");
                return;
            }
            if (!hasRole(user.role(), minRole)) {
                audit(exchange, user, "risk-api", "access.denied", 403, "{\"required\":\"" + minRole + "\"}");
                respond(exchange, 403, "{\"error\":\"forbidden\"}");
                return;
            }
            try {
                handler.handle(exchange, user);
            } finally {
                audit(exchange, user, "risk-api", routeAction(exchange), 200, "{}");
            }
        };
    }

    private void health(HttpExchange exchange) throws IOException {
        respond(exchange, 200, "{\"ok\":true,\"service\":\"risk-api\",\"uptimeSeconds\":" + uptimeSeconds() + "}");
    }

    private void metrics(HttpExchange exchange) throws IOException {
        String body;
        try (Connection conn = connect()) {
            int openLimits = count(conn, "SELECT COUNT(*) FROM accounts WHERE status = 'open'");
            int pendingTransfers = count(conn, "SELECT COUNT(*) FROM transfers WHERE status = 'pending'");
            body = "# HELP treasury_risk_open_accounts Open accounts\n"
                + "# TYPE treasury_risk_open_accounts gauge\n"
                + "treasury_risk_open_accounts " + openLimits + "\n"
                + "# HELP treasury_risk_pending_transfers Pending transfers\n"
                + "# TYPE treasury_risk_pending_transfers gauge\n"
                + "treasury_risk_pending_transfers " + pendingTransfers + "\n";
        } catch (SQLException exc) {
            respond(exchange, 500, "{\"error\":\"db_error\"}");
            return;
        }
        Headers headers = exchange.getResponseHeaders();
        headers.add("Content-Type", "text/plain; charset=utf-8");
        write(exchange, 200, body);
    }

    private void riskExposure(HttpExchange exchange, User user) throws IOException {
        if (!"GET".equals(exchange.getRequestMethod())) {
            respond(exchange, 405, "{\"error\":\"method_not_allowed\"}");
            return;
        }
        try (Connection conn = connect()) {
            String exposure = queryExposure(conn, user.organizationId());
            respond(exchange, 200, exposure);
        } catch (SQLException exc) {
            respond(exchange, 500, "{\"error\":\"db_error\"}");
        }
    }

    private void riskLimits(HttpExchange exchange, User user) throws IOException {
        if (!"GET".equals(exchange.getRequestMethod())) {
            respond(exchange, 405, "{\"error\":\"method_not_allowed\"}");
            return;
        }
        try (Connection conn = connect()) {
            String limits = queryLimits(conn, user.organizationId());
            respond(exchange, 200, limits);
        } catch (SQLException exc) {
            respond(exchange, 500, "{\"error\":\"db_error\"}");
        }
    }

    private void riskAudit(HttpExchange exchange, User user) throws IOException {
        if (!"POST".equals(exchange.getRequestMethod())) {
            respond(exchange, 405, "{\"error\":\"method_not_allowed\"}");
            return;
        }
        audit(exchange, user, "risk-api", "risk.manual_review", 202, "{\"source\":\"manual\"}");
        respond(exchange, 202, "{\"status\":\"queued\",\"organization_id\":\"" + escape(user.organizationId()) + "\"}");
    }

    private User authenticate(HttpExchange exchange) {
        String userId = exchange.getRequestHeaders().getFirst("X-User-Id");
        if (userId == null || userId.isBlank()) {
            userId = "u-alice";
        }
        try (Connection conn = connect();
             PreparedStatement stmt = conn.prepareStatement(
                 "SELECT id, organization_id, team_id, email, name, role FROM users WHERE id = ?")) {
            stmt.setString(1, userId);
            try (ResultSet rows = stmt.executeQuery()) {
                if (!rows.next()) {
                    return null;
                }
                return new User(
                    rows.getString("id"),
                    rows.getString("organization_id"),
                    rows.getString("team_id"),
                    rows.getString("email"),
                    rows.getString("name"),
                    rows.getString("role")
                );
            }
        } catch (SQLException exc) {
            return null;
        }
    }

    private String queryExposure(Connection conn, String orgId) throws SQLException {
        List<String> rows = new ArrayList<>();
        try (PreparedStatement stmt = conn.prepareStatement(
            "SELECT a.id, a.name, a.balance_cents, "
                + "COALESCE(SUM(CASE WHEN t.status = 'pending' THEN t.amount_cents ELSE 0 END), 0) AS pending_cents "
                + "FROM accounts a LEFT JOIN transfers t ON t.source_account_id = a.id AND t.organization_id = a.organization_id "
                + "WHERE a.organization_id = ? GROUP BY a.id, a.name, a.balance_cents ORDER BY a.name")) {
            stmt.setString(1, orgId);
            try (ResultSet rs = stmt.executeQuery()) {
                while (rs.next()) {
                    rows.add("{\"account_id\":\"" + escape(rs.getString("id")) + "\","
                        + "\"name\":\"" + escape(rs.getString("name")) + "\","
                        + "\"balance_cents\":" + rs.getLong("balance_cents") + ","
                        + "\"pending_transfer_cents\":" + rs.getLong("pending_cents") + "}");
                }
            }
        }
        return "{\"organization_id\":\"" + escape(orgId) + "\",\"exposures\":[" + String.join(",", rows) + "]}";
    }

    private String queryLimits(Connection conn, String orgId) throws SQLException {
        long transferCount = 0;
        long pendingTotal = 0;
        try (PreparedStatement stmt = conn.prepareStatement(
            "SELECT COUNT(*) AS transfer_count, COALESCE(SUM(amount_cents), 0) AS pending_total "
                + "FROM transfers WHERE organization_id = ? AND status = 'pending'")) {
            stmt.setString(1, orgId);
            try (ResultSet rs = stmt.executeQuery()) {
                if (rs.next()) {
                    transferCount = rs.getLong("transfer_count");
                    pendingTotal = rs.getLong("pending_total");
                }
            }
        }
        String state = pendingTotal > 5000000 ? "review_required" : "within_limit";
        return "{\"organization_id\":\"" + escape(orgId) + "\",\"pending_transfer_count\":" + transferCount
            + ",\"pending_transfer_cents\":" + pendingTotal + ",\"state\":\"" + state + "\"}";
    }

    private void audit(HttpExchange exchange, User user, String service, String action, int status, String metadata) {
        try (Connection conn = connect();
             PreparedStatement stmt = conn.prepareStatement(
                 "INSERT INTO audit_records "
                     + "(id, organization_id, actor_user_id, service, action, method, path, status_code, metadata) "
                     + "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)")) {
            URI uri = exchange.getRequestURI();
            stmt.setString(1, "audit-" + UUID.randomUUID());
            stmt.setString(2, user.organizationId());
            stmt.setString(3, user.id());
            stmt.setString(4, service);
            stmt.setString(5, action);
            stmt.setString(6, exchange.getRequestMethod());
            stmt.setString(7, uri.getPath());
            stmt.setInt(8, status);
            stmt.setString(9, metadata);
            stmt.executeUpdate();
        } catch (SQLException ignored) {
            // Audit must not break the API response path.
        }
    }

    private Connection connect() throws SQLException {
        return DriverManager.getConnection(dbUrl);
    }

    private int count(Connection conn, String sql) throws SQLException {
        try (PreparedStatement stmt = conn.prepareStatement(sql);
             ResultSet rs = stmt.executeQuery()) {
            return rs.next() ? rs.getInt(1) : 0;
        }
    }

    private String routeAction(HttpExchange exchange) {
        String path = exchange.getRequestURI().getPath();
        if (path.endsWith("/exposure")) {
            return "risk.exposure";
        }
        if (path.endsWith("/limits")) {
            return "risk.limits";
        }
        return "risk.request";
    }

    private boolean hasRole(String actual, String required) {
        return rank(actual) >= rank(required);
    }

    private int rank(String role) {
        return switch (String.valueOf(role).toLowerCase(Locale.ROOT)) {
            case "admin" -> 4;
            case "treasury_manager" -> 3;
            case "operator" -> 2;
            case "viewer" -> 1;
            default -> 0;
        };
    }

    private long uptimeSeconds() {
        return Math.max(0, (System.currentTimeMillis() - startedAt) / 1000);
    }

    private void respond(HttpExchange exchange, int status, String body) throws IOException {
        exchange.getResponseHeaders().add("Content-Type", "application/json; charset=utf-8");
        write(exchange, status, body);
    }

    private void write(HttpExchange exchange, int status, String body) throws IOException {
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        exchange.sendResponseHeaders(status, bytes.length);
        try (OutputStream output = exchange.getResponseBody()) {
            output.write(bytes);
        }
    }

    private String escape(String value) {
        return String.valueOf(value)
            .replace("\\", "\\\\")
            .replace("\"", "\\\"");
    }

    @FunctionalInterface
    private interface AuthedHandler {
        void handle(HttpExchange exchange, User user) throws IOException;
    }

    private record User(String id, String organizationId, String teamId, String email, String name, String role) {}
}
