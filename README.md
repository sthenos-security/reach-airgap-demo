# REACHABLE — Air-Gap Demo

### ▶ [Open the demo status page](https://sthenos-security.github.io/reach-airgap-demo/)

**What this shows:** REACHABLE found a real security bug in an application and
an AI coding agent fixed it, **on a computer that cannot reach the internet**.
A second scan confirmed the fix.

## See it

| Page | What it shows |
|---|---|
| **[Demo status page](https://sthenos-security.github.io/reach-airgap-demo/)** | The whole run: the isolation test, the firewall rules, the install, the scan, and the fix |
| [Before: what the scan found](https://sthenos-security.github.io/reach-airgap-demo/evidence/before.html) | The findings before the agent touched the code |
| [After: what was fixed](https://sthenos-security.github.io/reach-airgap-demo/evidence/fixed.html) | Each finding and its outcome: fixed, or still present |
| [Side-by-side code change](https://sthenos-security.github.io/reach-airgap-demo/evidence/code-diff-viewer.html) | The exact lines the agent changed |
| [Code before](https://sthenos-security.github.io/reach-airgap-demo/evidence/before-code-viewer.html) · [Code after](https://sthenos-security.github.io/reach-airgap-demo/evidence/after-code-viewer.html) | Browse the application at each step |

Every page is generated from the run's own records, not written by hand. The
"Generated" time on the status page is when that run finished.

## What "air-gapped" means here

An air-gapped network has **no road to the internet**. Banks, defense and
critical-infrastructure teams build networks like this so that nothing inside
can leak out and nothing outside can reach in. Most security tools quietly stop
working there, because they expect to download updates and call online
services.

In this demo, the protected network (the *enclave*) has no route out at all. A
live test inside it tries to open a public website and fails; the status page
shows that test.

## How it works

```
   STHENOS (internet)                    MODEL PROVIDER (internet)
   signed REACHABLE software             the AI model the coding
   + daily security intelligence         agent asks for fixes
            │                                       ▲
            ▼                                       │
┌─ MIRROR ─────────────────────────────────────────────────────────────────┐
│ The only machine that touches both the internet and the enclave.         │
│ Its firewall blocks everything except two services:                      │
│ Sthenos (software + intelligence) and the model provider.                │
└──────────────────────────────────────────────────────────────────────────┘
            │ software + intelligence in            ▲ fix requests out,
            ▼ (signature-checked)                   │ answers back
┌─ ISOLATED ENCLAVE (no route to the internet) ────────────────────────────┐
│ No route to the internet. A live test inside it fails.                   │
│                                                                          │
│ ┌──────────────────┐   reads packages    ┌──────────────────┐            │
│ │ REACHABLE        │ ──────────────────► │ PRIVATE PACKAGE  │            │
│ │ scanner          │                     │ REGISTRY         │            │
│ └────────┬─────────┘                     │ (no internet)    │            │
│          │ findings                      └──────────────────┘            │
│          ▼                                                               │
│ ┌──────────────────┐   asks the model only via the mirror (▲ above)      │
│ │ CODING AGENT     │ ──────────────────────────────────────►             │
│ │ fixes the code   │                                                     │
│ └────────┬─────────┘                                                     │
│          │ re-scan proves the fix                                        │
└──────────────────────────────────────────────────────────────────────────┘
            │
            ▼
   STATUS PAGES + EVIDENCE  →  https://sthenos-security.github.io/reach-airgap-demo/
```

Step by step:

1. **Sealed software arrives.** REACHABLE comes as a single package sealed with
   two digital signatures, one of them quantum-resistant. The enclave checks
   both seals before installing anything. A tampered package is refused.
2. **Security knowledge arrives the same way.** Today's vulnerability data and
   the license travel through the *mirror*, signed, so the enclave never has to
   go online to get them.
3. **The application's building blocks come from inside.** The application
   uses third-party packages. The enclave gets them only from a *private
   package registry* with no internet connection.
4. **REACHABLE scans the application.** It reports what is actually
   exploitable, not every theoretical issue. In this run: 2 findings, 1 of them
   exploitable.
5. **An AI coding agent fixes the code.** The agent needs an AI model. Its only
   way to reach one is through the mirror, whose firewall allows exactly that
   one model service and nothing else.
6. **A second scan proves the fix.** The exploitable finding is gone: 0 left.
7. **The results are published** to the pages above.

## Security measures

| Measure | What it protects against |
|---|---|
| **No route out of the enclave** | Code or data leaking to the internet. Proven by a live test on every run. |
| **One gatekeeper (the mirror) with a firewall** | Anything else slipping through. Its firewall blocks all traffic except two named destinations: Sthenos updates and the AI model. The rules are printed on the status page. |
| **Double-signed software** | A tampered or fake package. Two signatures are checked before install; one is quantum-resistant. |
| **Private package registry** | Pulling unknown code from the internet. It only serves files placed on it in advance and cannot fetch anything new. |
| **Signed security data and license** | Stale or forged vulnerability data. Older data is refused, so nobody can roll it back. |
| **Nothing secret in this repository** | Keys, passwords and network details stay on the private operator side. This repository holds only the sample application and the published pages. |

## Words used on these pages

- **Enclave** — the protected computer or network with no route to the internet.
- **Mirror** — the single gatekeeper machine between the enclave and the outside.
- **Private package registry** — the enclave's internal store of approved software components.
- **Finding** — a security issue the scan reports.
- **Exploitable / actionable** — a finding an attacker could actually reach and use, as opposed to one in code that never runs.
- **Remediation** — fixing the code so the finding goes away.
- **Signature** — a cryptographic seal that proves who built a file and that nobody changed it.

## What is in this repository

- **`app/`** — the sample application: a small fictional treasury system with
  four services in four languages. It contains deliberate security bugs.
- **`docs/`** — the published demo pages (served at the links above).

## Learn more

- Product: [sthenosec.com/airgap](https://sthenosec.com/airgap)
- Talk to us: [sales@sthenosec.com](mailto:sales@sthenosec.com)

---

© 2026 Sthenos Security. REACHABLE and the REACHABLE methodology are proprietary.
