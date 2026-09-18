import { CONFIG } from "./config.js";

const COLLECTION = "students";
const $status = document.getElementById("status");
const $count = document.getElementById("count");
const $raw = document.getElementById("raw");
const $lastMethod = document.getElementById("last-method");
const $getUrl = document.getElementById("get-url");
const $tableBody = document.getElementById("table-body");
const $studentForm = document.getElementById("student-form");
const $submitBtn = document.getElementById("submit-btn");
const $refresh = document.getElementById("refresh");
const $filterMajor = document.getElementById("filter-major");
const $filterName = document.getElementById("filter-name");

let loadSeq = 0;
const thumbCache = new Map();

function apiKeyHeaders() {
  return { "X-Nimbus-Key": CONFIG.apiKey };
}

function headers() {
  return { ...apiKeyHeaders(), "Content-Type": "application/json" };
}

function showRaw(method, data) {
  $lastMethod.textContent = method;
  $lastMethod.className = `tag ${method.toLowerCase()}`;
  $raw.textContent = JSON.stringify(data, null, 2);
}

function showStatus(msg, ok = false) {
  $status.hidden = false;
  $status.className = ok ? "status ok" : "status error";
  $status.textContent = msg;
  if (ok) window.setTimeout(() => { $status.hidden = true; }, 3500);
}

function buildQuery() {
  const params = new URLSearchParams();
  params.set("order", "created_at.desc");
  const major = $filterMajor.value.trim();
  const name = $filterName.value.trim();
  if (major) params.set("major", `eq.${major}`);
  if (name) params.set("name", `like.*${name}*`);
  return `/api/v1/data/${COLLECTION}?${params}`;
}

function apiError(data, statusText) {
  const err = data?.error;
  if (!err) return statusText;
  const msg = err.message || "";
  if (msg && msg !== err.code) return msg;
  if (err.code === "conflict") return "A file with this name already exists in the folder.";
  return err.code || statusText;
}

async function api(path, opts = {}) {
  const base = CONFIG.apiBase || "";
  const res = await fetch(`${base}${path}`, {
    ...opts,
    headers: { ...headers(), ...opts.headers },
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(apiError(data, res.statusText));
  return data;
}

async function postUpload(file) {
  const base = CONFIG.apiBase || "";
  const form = new FormData();
  const unique = new File([file], `${Date.now()}-${file.name}`, { type: file.type });
  form.append("file", unique);
  if (CONFIG.folderId) form.append("parent_id", CONFIG.folderId);
  const res = await fetch(`${base}/api/v1/files/upload`, {
    method: "POST",
    headers: apiKeyHeaders(),
    body: form,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(apiError(data, res.statusText));
  return data;
}

async function thumbUrl(fileId) {
  if (!fileId) return null;
  if (thumbCache.has(fileId)) return thumbCache.get(fileId);
  const base = CONFIG.apiBase || "";
  const res = await fetch(`${base}/api/v1/files/${fileId}/thumb`, {
    headers: apiKeyHeaders(),
  });
  if (!res.ok) return null;
  const url = URL.createObjectURL(await res.blob());
  thumbCache.set(fileId, url);
  return url;
}

function avatarCell(item, imgSrc) {
  const td = document.createElement("td");
  if (imgSrc) {
    const img = document.createElement("img");
    img.className = "avatar";
    img.src = imgSrc;
    img.alt = "";
    td.appendChild(img);
  } else {
    const span = document.createElement("span");
    span.className = "avatar-fallback";
    span.textContent = (item.name?.[0] || "?").toUpperCase();
    td.appendChild(span);
  }
  return td;
}

function textCell(text) {
  const td = document.createElement("td");
  td.textContent = text ?? "—";
  return td;
}

function actionCell(item) {
  const td = document.createElement("td");
  td.className = "actions";

  const btnJson = document.createElement("button");
  btnJson.type = "button";
  btnJson.className = "ghost sm";
  btnJson.textContent = "JSON";
  btnJson.onclick = () => showRaw("GET", item);

  const btnEdit = document.createElement("button");
  btnEdit.type = "button";
  btnEdit.className = "ghost sm";
  btnEdit.textContent = "PATCH";
  btnEdit.onclick = async () => {
    const name = window.prompt("Full name", item.name || "");
    if (name == null) return;
    const student_id = window.prompt("Student ID", item.student_id || "");
    if (student_id == null) return;
    const email = window.prompt("Email", item.email || "");
    if (email == null) return;
    const major = window.prompt("Major", item.major || "");
    if (major == null) return;
    const gpaStr = window.prompt("GPA", String(item.gpa ?? ""));
    if (gpaStr == null) return;
    try {
      const data = await api(`/api/v1/data/${COLLECTION}/${encodeURIComponent(item.id)}`, {
        method: "PATCH",
        body: JSON.stringify({
          name: name.trim(),
          student_id: student_id.trim(),
          email: email.trim(),
          major: major.trim(),
          gpa: Number(gpaStr),
        }),
      });
      showRaw("PATCH", data);
      showStatus(`PATCH OK — ${data.name}`, true);
      await load();
    } catch (err) {
      showStatus(err.message);
      showRaw("PATCH", { error: err.message });
    }
  };

  const btnDel = document.createElement("button");
  btnDel.type = "button";
  btnDel.className = "danger sm";
  btnDel.textContent = "DELETE";
  btnDel.onclick = async () => {
    if (!window.confirm(`Delete student "${item.name}"?`)) return;
    try {
      const data = await api(`/api/v1/data/${COLLECTION}/${encodeURIComponent(item.id)}`, {
        method: "DELETE",
      });
      showRaw("DELETE", data);
      showStatus(`DELETE OK — ${item.name}`, true);
      thumbCache.delete(item.photo_file_id);
      await load();
    } catch (err) {
      showStatus(err.message);
      showRaw("DELETE", { error: err.message });
    }
  };

  td.append(btnJson, btnEdit, btnDel);
  return td;
}

function row(item, imgSrc) {
  const tr = document.createElement("tr");
  tr.append(
    avatarCell(item, imgSrc),
    textCell(item.name),
    textCell(item.student_id),
    textCell(item.email),
    textCell(item.major),
    textCell(item.gpa != null ? Number(item.gpa).toFixed(2) : "—"),
    actionCell(item),
  );
  return tr;
}

async function load() {
  const seq = ++loadSeq;
  $status.hidden = true;
  $tableBody.innerHTML = `<tr><td colspan="7" class="empty-cell">Loading…</td></tr>`;
  $count.textContent = "";

  const path = buildQuery();
  $getUrl.textContent = path.replace("/api/v1", "GET /api/v1");

  try {
    const data = await api(path);
    if (seq !== loadSeq) return;

    const items = data.items || [];
    showRaw("GET", data);
    $count.textContent = `(${items.length})`;

    if (!items.length) {
      $tableBody.innerHTML = `<tr><td colspan="7" class="empty-cell">No students. POST one above.</td></tr>`;
      return;
    }

    $tableBody.replaceChildren();
    const frag = document.createDocumentFragment();
    for (const item of items) {
      if (seq !== loadSeq) return;
      const img = await thumbUrl(item.photo_file_id);
      if (seq !== loadSeq) return;
      frag.appendChild(row(item, img));
    }
    $tableBody.appendChild(frag);
  } catch (e) {
    if (seq !== loadSeq) return;
    $tableBody.innerHTML = `<tr><td colspan="7" class="empty-cell">Error: ${e.message}</td></tr>`;
    showStatus(`${e.message} — is Nimbus running? (go run ./cmd/nimbus)`);
    showRaw("GET", { error: e.message });
  }
}

$refresh.addEventListener("click", load);
$filterMajor.addEventListener("change", load);
$filterName.addEventListener("keydown", (e) => {
  if (e.key === "Enter") load();
});

$studentForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  const payload = {
    name: document.getElementById("f-name").value.trim(),
    student_id: document.getElementById("f-student-id").value.trim(),
    email: document.getElementById("f-email").value.trim(),
    major: document.getElementById("f-major").value.trim(),
    gpa: Number(document.getElementById("f-gpa").value),
  };
  const file = document.getElementById("f-photo").files?.[0];
  if (!payload.name || !payload.student_id) return;

  $submitBtn.disabled = true;
  $submitBtn.textContent = "Posting…";
  try {
    if (file) {
      const uploaded = await postUpload(file);
      payload.photo_file_id = uploaded.id;
      payload.photo_url = uploaded.url || null;
      showRaw("POST upload", uploaded);
    }
    const data = await api(`/api/v1/data/${COLLECTION}`, {
      method: "POST",
      body: JSON.stringify(payload),
    });
    showRaw("POST", data);
    showStatus(`POST OK — ${data.name}`, true);
    $studentForm.reset();
    await load();
  } catch (err) {
    showStatus(err.message);
    showRaw("POST", { error: err.message });
  } finally {
    $submitBtn.disabled = false;
    $submitBtn.textContent = "POST student";
  }
});

load();
