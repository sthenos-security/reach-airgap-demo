# Sthenos Security — Air-Gap Demonstration

**Public.** This repository is the demonstration that Sthenos Reachable performs
real application-security work — dependency and code vulnerability analysis,
reachability triage, and AI-assisted remediation — on a host with **no route to
the internet**, from a **cryptographically signed bundle**, using the
**customer's own internal package registry**.

It is the problem every regulated, classified, or disconnected environment hits:
modern security tooling assumes internet access for feeds, databases, and model
APIs. In an air-gapped enclave it has none — and most tools silently degrade to
scanning nothing while reporting "clean." Reachable is built for that
environment, and this demonstration proves it end to end.

## See it

- **Status pages (public):** **`https://sthenosec.com/airgap`** — two views,
  generated from an actual run (not hand-written):
  - **Air-Gap Status** — the environment is isolated (a live network test to a
    public site fails: no route out), the signed bundle installed offline, the
    **private package registry** that was stood up and served the app's
    dependencies, and the scan.
  - **Remediation Status** — the findings, the fixes the AI agent applied, and a
    re-scan confirming every exploitable issue is resolved.
- **Full scan results & dashboard (account console):** **`https://sthenosec.com`**
  — sign in to the **`sthenos-airgap-demo`** account. This is where the enclave's
  scan and remediation land after the run publishes its evidence out, exactly as a
  connected customer would see their own scans. The status pages above are the
  public summary; the dashboard is the full, per-finding console.

## High-level architecture (no secrets, no addresses)

```
          ┌──────────────────────────── ISOLATED ENCLAVE ────────────────────────────┐
          │  no route to the internet (a live network test proves it)                 │
          │                                                                           │
   signed │   ┌───────────────┐      resolves deps from                              │
 bundle + │   │   REACHABLE    │ ───────────────────────────►  ┌────────────────────┐ │
  intel   │   │  scan +        │                               │ PRIVATE package    │ │
  ───────►│   │  remediation   │ ◄───────────────────────────  │ registry (customer │ │
   (one   │   │  (offline)     │      the app's own closure    │ operated, no egress)│ │
   way)   │   └───────┬───────┘                                └────────────────────┘ │
          │           │ redacted, signed evidence (allowlisted — no secrets)           │
          └───────────┼───────────────────────────────────────────────────────────────┘
                      │  ONE-WAY mirror lane (signed in, redacted evidence out;
                      ▼  never a path INTO the enclave)
            ┌───────────────────┐        ┌──────────────────────────┐
            │  customer mirror  │ ─────► │  Sthenos cloud → dashboard │  (sthenos-airgap-demo)
            └───────────────────┘        └──────────────────────────┘
```

- **The enclave never reaches the internet.** Intelligence and the signed product
  bundle come *in* over one controlled lane; redacted evidence goes *out* over the
  same lane. No inbound path is ever opened.
- **The customer's own registry** serves the application's dependencies inside the
  enclave — the scanner never calls the public internet.
- **Everything that crosses is signed and verified**, and the evidence that leaves
  is allowlisted — no credentials, no internal addresses, no topology.
- Keys, endpoints, and topology are deliberately **not** shown here; they live on
  the private operator side.

## What is proven

| Claim | How it is shown |
|---|---|
| **Zero internet egress** | A live network test inside the enclave fails — no route out — and independent controls record zero outbound connection attempts during the run. |
| **Trusted supply chain** | Everything that crosses the gap is a signed bundle, **signature-verified** and delivered **encrypted**, re-checked inside the enclave before anything runs. |
| **Customer's own registry** | A private package registry is stood up inside the enclave; the scanner resolves every dependency from it, never from the public internet. |
| **Reachability, not noise** | Findings are triaged by real exploitability — the exploitable few are separated from the inert many, so scarce review time goes where risk actually is. |
| **Closed-loop remediation** | An AI coding agent, itself offline, fixes the exploitable findings; a re-scan proves the result. |
| **Evidence, not secrets** | Each run emits an allowlisted evidence record — no credentials, no internal addresses — safe to publish and to carry back to the customer's dashboard. |

## What's in this repo

- **`app/`** — a realistic multi-service treasury/payments application spanning
  several languages and package ecosystems, so the registry and reachability
  work is genuine.
- **`ground-truth.json`** — the expected result the enclave scan must reproduce.
  It is **generated by a scan, never hand-written**, so "the scan matches the
  truth" is a real check.
- **`docs/`** — the published status pages (served at the link above).

This repository holds **no credentials, no network topology, and no operational
tooling** — those live on the private side. It is only the thing that crosses
the gap and the public result.

## The one disclosed caveat

For the demonstration, the **local AI model is simulated** — it is the single
component not running for real. Everything else (mirror, offline install, scan,
zero-egress proof, re-scan) is live. In a customer deployment the model is
replaced by the customer's own on-premises model on the same offline lane — a
configuration change, not a new architecture. The status page states this
plainly rather than implying a capability that is mocked.

## Keys — how this demo differs from a customer

A real customer generates their mirror + enclave-host keys **through the Sthenos
dashboard**, with only a single firewall hole to `sthenosec.com` (no internet):
- **Model B (production):** the dashboard generates the Ed25519 keypairs **in the
  browser** and the customer downloads the private halves — the private key never
  reaches Sthenos, so Sthenos cannot impersonate their mirror or enclave.
- **"Generate temporary test credentials" (evaluation):** a server-side, 30/60/90-day
  option so a customer can try the full flow before provisioning their own keys.

**This demo does neither.** Because it runs against the Sthenos-owned demo org, it
**self-generates its keys in CI (a preflight-auth step)** — a convenience for our own
org only, never the customer path. Full procedure + required dashboard changes:
reach-core `docs/RUNBOOK-AIRGAP-ONBOARDING.md` §3–§3c.

---

Architecture, assurance, and deployment questions: contact Sthenos Security.
© 2026 Sthenos Security. Reachable and the Reachable methodology are proprietary.
