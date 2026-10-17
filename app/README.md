# Fictional Treasury Operations Portal

Small multi-service treasury SaaS demo with:

- `portal/`: TypeScript admin portal, JavaScript dashboard, and thin JSON API.
- `payments-api/`: Go API for accounts, transfers, settlement jobs, receipts, health, and metrics.
- `docs-worker/`: Python WSGI service for document upload/download and background indexing.
- `risk-api/`: Java HTTP service for risk exposure, limits, audit writes, health, and metrics.
- `data/treasury.db`: local SQLite database created automatically by any service.

## Seed Users

Pass these headers to act as a seeded user:

- `X-User-Id: u-alice` admin in `org-northstar`
- `X-User-Id: u-ben` treasury operator in `org-northstar`
- `X-User-Id: u-chen` viewer in `org-northstar`

## Run Locally

Install and run the portal:

```sh
cd portal
npm install
npm run dev
```

Run the payments API:

```sh
cd payments-api
go mod download
go run .
```

Run the docs worker:

```sh
cd docs-worker
python3 app.py
```

Compile and run the risk API:

```sh
cd risk-api
javac -d build/classes src/com/sthenos/treasury/RiskApi.java
java -cp build/classes com.sthenos.treasury.RiskApi
```

Default ports:

- Portal: `http://localhost:3000`
- Payments API: `http://localhost:8080`
- Docs worker: `http://localhost:8090`
- Risk API: `http://localhost:8070`

## Example API Calls

Create a transfer:

```sh
curl -X POST http://localhost:8080/transfers \
  -H 'Content-Type: application/json' \
  -H 'X-User-Id: u-ben' \
  -d '{"source_account_id":"acct-operating","beneficiary_id":"ben-atlas","amount_cents":125000,"currency":"USD","memo":"Vendor settlement"}'
```

Upload a document:

```sh
curl -X POST http://localhost:8090/documents \
  -H 'X-User-Id: u-ben' \
  -H 'X-File-Name: invoice.txt' \
  --data-binary 'invoice bytes'
```

Fetch risk exposure:

```sh
curl http://localhost:8070/risk/exposure \
  -H 'X-User-Id: u-alice'
```
