"use strict";

const $ = (id) => document.getElementById(id);
const money = (n) => new Intl.NumberFormat("ru-RU").format(n) + " ₸";
// Trips carry their own UTC offset; show the wall-clock time from the string
// itself rather than converting into the browser timezone.
const hhmm = (iso) => iso.slice(11, 16);

const PAYMENT_LABEL = { cash: "Наличные", card: "Карта" };

let currentDate = todayISO();

function todayISO() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// Convert a datetime-local value into an RFC3339 string with the browser's
// UTC offset, which the server requires.
function toRFC3339(local) {
  const d = new Date(local);
  const pad = (n) => String(n).padStart(2, "0");
  const off = -d.getTimezoneOffset();
  const sign = off >= 0 ? "+" : "-";
  const abs = Math.abs(off);
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}:00` +
    `${sign}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`
  );
}

function shiftDay(days) {
  const d = new Date(currentDate + "T00:00:00");
  d.setDate(d.getDate() + days);
  const pad = (n) => String(n).padStart(2, "0");
  currentDate = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  $("date").value = currentDate;
  refresh();
}

// Fetch JSON and turn non-2xx answers into errors with the server's message.
async function getJSON(url) {
  const res = await fetch(url);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

// Incremented on every refresh so that a slow response for a day the user has
// already left does not overwrite the current one.
let refreshSeq = 0;

async function refresh() {
  const seq = ++refreshSeq;
  const date = currentDate;
  try {
    const [summary, trips] = await Promise.all([
      getJSON(`/api/summary?date=${date}`),
      getJSON(`/api/trips?date=${date}`),
    ]);
    if (seq !== refreshSeq) return;
    $("load-error").hidden = true;
    renderSummary(summary);
    renderTrips(trips);
  } catch (err) {
    if (seq !== refreshSeq) return;
    $("load-error").hidden = false;
    $("load-error").textContent = "Не удалось загрузить данные: " + err.message;
  }
}

function renderSummary(s) {
  $("s-trips").textContent = s.trips;
  $("s-revenue").textContent = money(s.revenue);
  $("s-net").textContent = money(s.net);
  $("s-commission").textContent = money(s.commission);
  $("s-cash").textContent = `${money(s.cash.amount)} · ${s.cash.trips}`;
  $("s-card").textContent = `${money(s.card.amount)} · ${s.card.trips}`;
}

function renderTrips(trips) {
  const tbody = $("trips");
  tbody.innerHTML = "";
  $("trips-empty").hidden = trips.length > 0;
  for (const t of trips) {
    const tr = document.createElement("tr");
    tr.innerHTML = `
      <td>${hhmm(t.start)}</td>
      <td>${hhmm(t.end)}</td>
      <td><span class="pill ${t.payment}">${PAYMENT_LABEL[t.payment]}</span></td>
      <td class="num">${money(t.amount)}</td>
      <td class="num">${money(t.commission)}</td>`;
    tbody.appendChild(tr);
  }
}

async function submitTrip(e) {
  e.preventDefault();
  const msg = $("form-msg");
  msg.className = "msg";
  msg.textContent = "";

  const body = {
    start: toRFC3339($("f-start").value),
    end: toRFC3339($("f-end").value),
    amount: Number($("f-amount").value),
    commission: Number($("f-commission").value),
    payment: $("f-payment").value,
  };

  // Disable the button while sending so a double click cannot fire twice.
  const button = $("add-btn");
  button.disabled = true;
  let res, data;
  try {
    res = await fetch("/api/trips", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    data = await res.json().catch(() => ({}));
  } catch (err) {
    msg.className = "msg err";
    msg.textContent = "Сеть недоступна: " + err.message;
    return;
  } finally {
    button.disabled = false;
  }

  if (!res.ok) {
    msg.className = "msg err";
    msg.textContent = "Ошибка: " + (data.error || res.status);
    return;
  }
  msg.className = "msg ok";
  msg.textContent = res.status === 201 ? "Поездка добавлена" : "Такая поездка уже есть";
  $("f-amount").value = "";

  // Jump to the day of the added trip so the user sees the result.
  currentDate = data.start.slice(0, 10);
  $("date").value = currentDate;
  refresh();
}

function init() {
  $("date").value = currentDate;
  $("date").addEventListener("change", (e) => {
    currentDate = e.target.value;
    refresh();
  });
  $("prev").addEventListener("click", () => shiftDay(-1));
  $("next").addEventListener("click", () => shiftDay(1));
  $("add-form").addEventListener("submit", submitTrip);
  refresh();
}

init();
