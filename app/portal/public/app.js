const userSelect = document.querySelector("#userId");
const money = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD" });

userSelect.addEventListener("change", loadDashboard);
document.querySelector("#beneficiaryForm").addEventListener("submit", async (event) => {
  event.preventDefault();
  const body = Object.fromEntries(new FormData(event.currentTarget).entries());
  const response = await fetch("/api/beneficiaries", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-User-Id": userSelect.value
    },
    body: JSON.stringify(body)
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    alert(payload.error ?? "Unable to add beneficiary");
    return;
  }
  event.currentTarget.reset();
  loadDashboard();
});

async function loadDashboard() {
  const response = await fetch("/api/dashboard", { headers: { "X-User-Id": userSelect.value } });
  if (!response.ok) {
    document.querySelector("main").innerHTML = `<div class="panel"><div class="panel-header"><h2>Access denied</h2></div></div>`;
    return;
  }
  const data = await response.json();
  renderSummary(data);
  renderAccounts(data.accounts);
  renderBeneficiaries(data.beneficiaries);
  renderTransfers(data.transfers);
  renderAudit(data.auditRecords);
}

function renderSummary(data) {
  const totalBalance = data.accounts.reduce((sum, account) => sum + account.balance_cents, 0);
  const pending = data.transfers.filter((transfer) => transfer.status === "pending").length;
  document.querySelector("#summary").innerHTML = [
    metric("Organization", data.organization.name),
    metric("Users", String(data.users.length)),
    metric("Total Cash", money.format(totalBalance / 100)),
    metric("Pending Transfers", String(pending))
  ].join("");
}

function metric(label, value) {
  return `<div class="metric"><span>${escapeHtml(label)}</span><strong>${escapeHtml(value)}</strong></div>`;
}

function renderAccounts(accounts) {
  document.querySelector("#accounts").innerHTML = table(
    ["Name", "Bank", "Balance", "Status"],
    accounts.map((account) => [
      account.name,
      account.bank_name,
      money.format(account.balance_cents / 100),
      `<span class="pill">${account.status}</span>`
    ])
  );
}

function renderBeneficiaries(beneficiaries) {
  document.querySelector("#beneficiaries").innerHTML =
    beneficiaries
      .map(
        (beneficiary) => `
        <div class="item">
          <div>
            <strong>${escapeHtml(beneficiary.name)}</strong>
            <div class="subtle">${escapeHtml(beneficiary.bank_name)} ending ${escapeHtml(beneficiary.account_number_last4)}</div>
          </div>
          <span class="pill">${escapeHtml(beneficiary.currency)}</span>
        </div>`
      )
      .join("") || `<div class="subtle">No beneficiaries</div>`;
}

function renderTransfers(transfers) {
  document.querySelector("#transfers").innerHTML = table(
    ["Beneficiary", "Account", "Amount", "Status", "Requested"],
    transfers.map((transfer) => [
      transfer.beneficiary,
      transfer.source_account,
      money.format(transfer.amount_cents / 100),
      `<span class="pill">${escapeHtml(transfer.status)}</span>`,
      transfer.requested_by
    ])
  );
}

function renderAudit(records) {
  document.querySelector("#audit").innerHTML =
    records
      .map(
        (record) => `
        <div class="item">
          <div>
            <strong>${escapeHtml(record.action)}</strong>
            <div class="subtle">${escapeHtml(record.service)} ${escapeHtml(record.method)} ${escapeHtml(record.path)}</div>
          </div>
          <span class="subtle">${escapeHtml(record.status_code)}</span>
        </div>`
      )
      .join("") || `<div class="subtle">No audit records yet</div>`;
}

function table(headers, rows) {
  if (!rows.length) {
    return `<div class="list"><div class="subtle">No records</div></div>`;
  }
  return `<table><thead><tr>${headers.map((header) => `<th>${escapeHtml(header)}</th>`).join("")}</tr></thead><tbody>${rows
    .map((row) => `<tr>${row.map((cell) => `<td>${cell}</td>`).join("")}</tr>`)
    .join("")}</tbody></table>`;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

loadDashboard();

