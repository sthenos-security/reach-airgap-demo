# Treasury Risk API

Java HTTP service for the fictional treasury operations portal.

## Endpoints

- `GET /health`
- `GET /metrics`
- `GET /risk/exposure`
- `GET /risk/limits`
- `POST /risk/audit`

Authenticated endpoints use `X-User-Id` and the shared SQLite user table. If
the header is omitted, the service uses seeded admin user `u-alice`.

## Run

```sh
javac -d build/classes src/com/sthenos/treasury/RiskApi.java
java -cp build/classes com.sthenos.treasury.RiskApi
```

Environment variables:

- `PORT`: defaults to `8070`
- `TREASURY_DB_PATH`: defaults to `../data/treasury.db`
