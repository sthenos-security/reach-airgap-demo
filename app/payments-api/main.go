package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type ctxKey string

const userKey ctxKey = "user"

type app struct {
	db        *sql.DB
	startedAt time.Time
}

type user struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Role           string `json:"role"`
}

type transferRequest struct {
	SourceAccountID string `json:"source_account_id"`
	BeneficiaryID   string `json:"beneficiary_id"`
	AmountCents     int64  `json:"amount_cents"`
	Currency        string `json:"currency"`
	Memo            string `json:"memo"`
}

type auditWriter struct {
	http.ResponseWriter
	status int
}

func (w *auditWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func main() {
	root, err := findRoot()
	if err != nil {
		log.Fatal(err)
	}
	db, err := openDB(root)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	a := &app{db: db, startedAt: time.Now()}
	go a.settlementWorker(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /metrics", a.metrics)
	mux.Handle("GET /accounts", a.withMiddleware(http.HandlerFunc(a.accounts), "viewer"))
	mux.Handle("GET /beneficiaries", a.withMiddleware(http.HandlerFunc(a.beneficiaries), "viewer"))
	mux.Handle("GET /transfers", a.withMiddleware(http.HandlerFunc(a.transfers), "viewer"))
	mux.Handle("POST /transfers", a.withMiddleware(http.HandlerFunc(a.createTransfer), "operator"))
	mux.Handle("GET /transfers/{id}/receipt", a.withMiddleware(http.HandlerFunc(a.transferReceipt), "viewer"))

	port := getenv("PORT", "8080")
	log.Printf("payments api listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func findRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "scripts", "schema.sql")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("could not find repository root with scripts/schema.sql")
		}
		wd = parent
	}
}

func openDB(root string) (*sql.DB, error) {
	dbPath := getenv("TREASURY_DB_PATH", filepath.Join(root, "data", "treasury.db"))
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	schema, err := os.ReadFile(filepath.Join(root, "scripts", "schema.sql"))
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(string(schema)); err != nil {
		return nil, err
	}
	return db, nil
}

func (a *app) withMiddleware(next http.Handler, minRole string) http.Handler {
	return requestID(cors(a.authenticate(a.audit(a.authorize(next, minRole)))))
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", fmt.Sprintf("req-%d", time.Now().UnixNano()))
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-Id")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-Id")
		if userID == "" {
			userID = "u-alice"
		}
		u, err := a.lookupUser(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_user"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *app) authorize(next http.Handler, minRole string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if roleRank(u.Role) < roleRank(minRole) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden", "requiredRole": minRole})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aw := &auditWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(aw, r)
		u := currentUser(r)
		_, err := a.db.ExecContext(
			r.Context(),
			`INSERT INTO audit_records
			 (id, organization_id, actor_user_id, service, action, method, path, status_code, metadata)
			 VALUES (?, ?, ?, 'payments-api', ?, ?, ?, ?, ?)`,
			newID("audit"),
			u.OrganizationID,
			u.ID,
			actionFor(r),
			r.Method,
			r.URL.Path,
			aw.status,
			fmt.Sprintf(`{"remote_addr":%q}`, r.RemoteAddr),
		)
		if err != nil {
			log.Printf("audit insert failed: %v", err)
		}
	})
}

func (a *app) lookupUser(ctx context.Context, id string) (user, error) {
	var u user
	err := a.db.QueryRowContext(ctx,
		`SELECT id, organization_id, name, email, role
		   FROM users
		  WHERE id = ? AND active = 1`,
		id,
	).Scan(&u.ID, &u.OrganizationID, &u.Name, &u.Email, &u.Role)
	return u, err
}

func currentUser(r *http.Request) user {
	u, _ := r.Context().Value(userKey).(user)
	return u
}

func roleRank(role string) int {
	switch role {
	case "admin":
		return 4
	case "treasury_manager":
		return 3
	case "operator":
		return 2
	case "viewer":
		return 1
	default:
		return 0
	}
}

func actionFor(r *http.Request) string {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/transfers":
		return "transfer.create"
	case strings.HasSuffix(r.URL.Path, "/receipt"):
		return "transfer.receipt.download"
	default:
		return "request"
	}
}

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	var transfers int
	if err := a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM transfers").Scan(&transfers); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"service":        "payments-api",
		"transfers":      transfers,
		"uptime_seconds": int(time.Since(a.startedAt).Seconds()),
	})
}

func (a *app) metrics(w http.ResponseWriter, r *http.Request) {
	var pending, settled int
	_ = a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM transfers WHERE status = 'pending'").Scan(&pending)
	_ = a.db.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM transfers WHERE status = 'settled'").Scan(&settled)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# TYPE treasury_payments_pending_transfers gauge\n")
	fmt.Fprintf(w, "treasury_payments_pending_transfers %d\n", pending)
	fmt.Fprintf(w, "# TYPE treasury_payments_settled_transfers counter\n")
	fmt.Fprintf(w, "treasury_payments_settled_transfers %d\n", settled)
}

func (a *app) accounts(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, name, bank_name, currency, balance_cents, status
		   FROM accounts
		  WHERE organization_id = ?
		  ORDER BY name`,
		u.OrganizationID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items, err := rowsToMaps(rows)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": items})
}

func (a *app) beneficiaries(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT id, name, bank_name, account_number_last4, currency, status
		   FROM beneficiaries
		  WHERE organization_id = ?
		  ORDER BY name`,
		u.OrganizationID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items, err := rowsToMaps(rows)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"beneficiaries": items})
}

func (a *app) transfers(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT tr.id, tr.amount_cents, tr.currency, tr.status, tr.memo, tr.created_at, tr.settled_at,
		        a.name AS source_account, b.name AS beneficiary
		   FROM transfers tr
		   JOIN accounts a ON a.id = tr.source_account_id
		   JOIN beneficiaries b ON b.id = tr.beneficiary_id
		  WHERE tr.organization_id = ?
		  ORDER BY tr.created_at DESC`,
		u.OrganizationID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items, err := rowsToMaps(rows)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transfers": items})
}

func (a *app) createTransfer(w http.ResponseWriter, r *http.Request) {
	var input transferRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if input.SourceAccountID == "" || input.BeneficiaryID == "" || input.AmountCents <= 0 || input.Currency == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_required_fields"})
		return
	}

	u := currentUser(r)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	var balance int64
	var accountCurrency string
	err = tx.QueryRowContext(r.Context(),
		`SELECT balance_cents, currency
		   FROM accounts
		  WHERE id = ? AND organization_id = ? AND status = 'open'`,
		input.SourceAccountID,
		u.OrganizationID,
	).Scan(&balance, &accountCurrency)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_account_not_found"})
		return
	}
	if accountCurrency != input.Currency {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "currency_mismatch"})
		return
	}
	if balance < input.AmountCents {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "insufficient_funds"})
		return
	}

	var beneficiaryCount int
	err = tx.QueryRowContext(r.Context(),
		`SELECT COUNT(*)
		   FROM beneficiaries
		  WHERE id = ? AND organization_id = ? AND currency = ? AND status = 'active'`,
		input.BeneficiaryID,
		u.OrganizationID,
		input.Currency,
	).Scan(&beneficiaryCount)
	if err != nil || beneficiaryCount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "beneficiary_not_found"})
		return
	}

	transferID := newID("tr")
	jobID := newID("job")
	_, err = tx.ExecContext(r.Context(),
		`INSERT INTO transfers
		 (id, organization_id, source_account_id, beneficiary_id, amount_cents, currency, memo, status, requested_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)`,
		transferID,
		u.OrganizationID,
		input.SourceAccountID,
		input.BeneficiaryID,
		input.AmountCents,
		input.Currency,
		input.Memo,
		u.ID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, err = tx.ExecContext(r.Context(),
		`INSERT INTO settlement_jobs (id, organization_id, transfer_id, status)
		 VALUES (?, ?, ?, 'queued')`,
		jobID,
		u.OrganizationID,
		transferID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": transferID, "status": "pending", "settlement_job_id": jobID})
}

func (a *app) transferReceipt(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id := r.PathValue("id")
	row := a.db.QueryRowContext(r.Context(),
		`SELECT tr.id, tr.amount_cents, tr.currency, tr.status, tr.created_at, tr.settled_at,
		        a.name AS source_account, b.name AS beneficiary
		   FROM transfers tr
		   JOIN accounts a ON a.id = tr.source_account_id
		   JOIN beneficiaries b ON b.id = tr.beneficiary_id
		  WHERE tr.id = ? AND tr.organization_id = ?`,
		id,
		u.OrganizationID,
	)
	var transferID, currency, status, createdAt, source, beneficiary string
	var settledAt sql.NullString
	var amount int64
	if err := row.Scan(&transferID, &amount, &currency, &status, &createdAt, &settledAt, &source, &beneficiary); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "transfer_not_found"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-receipt.txt"`, transferID))
	fmt.Fprintf(w, "Treasury transfer receipt\n")
	fmt.Fprintf(w, "Transfer: %s\nStatus: %s\nAmount: %.2f %s\nSource: %s\nBeneficiary: %s\nCreated: %s\nSettled: %s\n",
		transferID,
		status,
		float64(amount)/100,
		currency,
		source,
		beneficiary,
		createdAt,
		nullText(settledAt),
	)
}

func (a *app) settlementWorker(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.processSettlementJob(ctx); err != nil && !errors.Is(err, sql.ErrNoRows) {
				log.Printf("settlement worker error: %v", err)
			}
		}
	}
}

func (a *app) processSettlementJob(ctx context.Context) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var jobID, orgID, transferID, sourceAccountID string
	var amount int64
	err = tx.QueryRowContext(ctx,
		`SELECT sj.id, sj.organization_id, sj.transfer_id, tr.source_account_id, tr.amount_cents
		   FROM settlement_jobs sj
		   JOIN transfers tr ON tr.id = sj.transfer_id
		  WHERE sj.status = 'queued'
		  ORDER BY sj.created_at
		  LIMIT 1`,
	).Scan(&jobID, &orgID, &transferID, &sourceAccountID, &amount)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx,
		`UPDATE settlement_jobs
		    SET status = 'processing', attempts = attempts + 1, updated_at = ?
		  WHERE id = ? AND status = 'queued'`,
		now,
		jobID,
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE accounts
		    SET balance_cents = balance_cents - ?
		  WHERE id = ? AND organization_id = ? AND balance_cents >= ?`,
		amount,
		sourceAccountID,
		orgID,
		amount,
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE transfers SET status = 'settled', settled_at = ? WHERE id = ? AND status = 'pending'`,
		now,
		transferID,
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE settlement_jobs SET status = 'done', updated_at = ? WHERE id = ?`,
		now,
		jobID,
	)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_records
		 (id, organization_id, actor_user_id, service, action, method, path, status_code, metadata)
		 VALUES (?, ?, NULL, 'payments-api', 'settlement.complete', 'WORKER', ?, 200, ?)`,
		newID("audit"),
		orgID,
		"/settlement-jobs/"+jobID,
		fmt.Sprintf(`{"transfer_id":%q}`, transferID),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func rowsToMaps(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		item := map[string]any{}
		for i, col := range cols {
			switch v := values[i].(type) {
			case []byte:
				item[col] = string(v)
			default:
				item[col] = v
			}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func nullText(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
