let strategy = "auto";
const lanes = {}; // provider name -> { el, fill, badge, chunks }

document.querySelectorAll(".pill").forEach(p => {
  p.addEventListener("click", () => {
    document.querySelectorAll(".pill").forEach(x => x.classList.remove("on"));
    p.classList.add("on");
    strategy = p.dataset.strategy;
  });
});

document.getElementById("run").addEventListener("click", run);

// esc escapes server-provided strings before they touch innerHTML.
// Inputs here are self-controlled (provider names, our own error text), but
// escaping keeps the timeline XSS-safe regardless.
function esc(s) {
  return String(s == null ? "" : s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function run() {
  // reset
  document.getElementById("lanes").innerHTML = "";
  document.getElementById("timeline").innerHTML = "";
  for (const k in lanes) delete lanes[k];
  ["st-in", "st-intent", "st-race", "st-out"].forEach(id => {
    const el = document.getElementById(id);
    el.classList.remove("ok", "bad");
    el.querySelector(".sv").textContent = "—";
    const m = el.querySelector(".masked");
    if (m) m.textContent = "";
  });
  document.getElementById("response").textContent = "";
  document.getElementById("resp-prov").textContent = "";

  const q = encodeURIComponent(document.getElementById("q").value);
  const es = new EventSource(`/viz/stream?q=${q}&strategy=${strategy}`);

  es.onmessage = (msg) => {
    if (msg.data === "[DONE]") { es.close(); return; }
    const e = JSON.parse(msg.data);
    handle(e);
  };
  es.onerror = () => es.close();
}

function ensureLane(name) {
  if (lanes[name]) return lanes[name];
  const wrap = document.createElement("div");
  wrap.className = "lane";
  wrap.innerHTML = `
    <span class="pname">${esc(name)}</span>
    <div class="track"><div class="fill"></div></div>
    <span class="badge">running</span>
    <span class="sab">
      <button data-mode="fail">💥</button>
      <button data-mode="delay">⏱ +5s</button>
      <button data-mode="clear">♻️</button>
    </span>`;
  document.getElementById("lanes").appendChild(wrap);
  const lane = {
    el: wrap,
    fill: wrap.querySelector(".fill"),
    badge: wrap.querySelector(".badge"),
    chunks: 0,
  };
  wrap.querySelectorAll(".sab button").forEach(b => {
    b.addEventListener("click", () => sabotage(name, b.dataset.mode));
  });
  lanes[name] = lane;
  return lane;
}

function tl(text) {
  const t = document.getElementById("timeline");
  t.innerHTML += text + "<br>";
}

function setStage(id, text, cls) {
  const el = document.getElementById(id);
  if (!el) return;
  el.querySelector(".sv").textContent = text;
  if (cls) el.classList.add(cls);
}

function handle(e) {
  switch (e.type) {
    case "guard_in":
      setStage("st-in", `mascarado: ${esc(e.detail)} ${esc(e.content)}`, "ok");
      tl(`t=${e.t}ms · guard_in: ${esc(e.detail)} → ${esc(e.content)}`);
      break;
    case "masked_prompt": {
      const stIn = document.getElementById("st-in");
      const m = stIn.querySelector(".masked");
      if (m) m.textContent = "prompt→LLM: " + e.content; // textContent is XSS-safe
      if (!stIn.classList.contains("bad")) stIn.classList.add("ok");
      break;
    }
    case "blocked":
      setStage("st-in", `BLOQUEADO: ${esc(e.detail)}`, "bad");
      tl(`<b>t=${e.t}ms</b> · BLOQUEADO: ${esc(e.detail)}`);
      break;
    case "intent":
      setStage("st-intent", esc(e.detail), "ok");
      tl(`t=${e.t}ms · intent: ${esc(e.detail)}`);
      break;
    case "guard_out":
      setStage("st-out", `${esc(e.detail)} ${esc(e.content)}`, "ok");
      tl(`t=${e.t}ms · guard_out: ${esc(e.detail)}`);
      break;
    case "out_chunk": {
      const rp = document.getElementById("resp-prov");
      if (!rp.textContent && e.provider) rp.textContent = "(" + e.provider + ")";
      // textContent is XSS-safe; the answer is also already PII-scrubbed.
      document.getElementById("response").textContent += e.content;
      break;
    }
    case "start":
      tl(`<b>t=${e.t}ms</b> · start (${esc(e.detail)})`);
      setStage("st-race", "correndo...", "ok");
      break;
    case "provider_start": {
      ensureLane(e.provider);
      tl(`t=${e.t}ms · ${esc(e.provider)} iniciou`);
      break;
    }
    case "chunk": {
      const lane = ensureLane(e.provider);
      lane.chunks++;
      const w = Math.min(90, lane.chunks * 12);
      lane.fill.style.width = w + "%";
      break;
    }
    case "won": {
      const lane = ensureLane(e.provider);
      lane.el.classList.add("won");
      lane.fill.style.width = "100%";
      lane.badge.textContent = "WON";
      tl(`<b>t=${e.t}ms</b> · ${esc(e.provider)} venceu`);
      break;
    }
    case "cancelled": {
      const lane = ensureLane(e.provider);
      lane.el.classList.add("cancelled");
      lane.badge.textContent = "cancelled ❌";
      tl(`t=${e.t}ms · ${esc(e.provider)} cancelado (ctx)`);
      break;
    }
    case "failed": {
      const lane = ensureLane(e.provider);
      lane.el.classList.add("failed");
      lane.badge.textContent = "failed";
      tl(`t=${e.t}ms · ${esc(e.provider)} falhou: ${esc(e.detail || "")}`);
      break;
    }
    case "decision":
      tl(`t=${e.t}ms · decisão: ${esc(e.detail)}`);
      break;
    case "done": {
      const lane = ensureLane(e.provider);
      lane.fill.style.width = "100%";
      if (!lane.el.classList.contains("won")) lane.el.classList.add("won");
      tl(`<b>t=${e.t}ms</b> · ${esc(e.provider)} concluiu`);
      break;
    }
    case "error":
      tl(`<b>t=${e.t}ms</b> · ERRO: ${esc(e.detail)}`);
      break;
  }
}

function sabotage(provider, mode) {
  fetch("/viz/sabotage", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, mode, delay_ms: 5000 }),
  });
}
