/* ============================================================
   PARALLEL GP — dashboard behavior
   SSE consumer for /viz/stream + start-light launch + sabotage.

   HARD RULES (see task brief):
   - Every server string reaches the DOM only via textContent.
     Never innerHTML with interpolated server data.
   - GSAP and confetti are OPTIONAL. Guard every use; pure-CSS fallback
     must keep the dashboard fully functional with no CDN.
   ============================================================ */

"use strict";

let strategy = "auto";
let eventCount = 0;
const lanes = {}; // provider name -> { el, trail, car, badge, telem, chunks }

const $ = (id) => document.getElementById(id);

/* ---------- strategy pills ---------- */
$("pills").addEventListener("click", (ev) => {
  const pill = ev.target.closest(".pill");
  if (!pill) return;
  document.querySelectorAll(".pill").forEach((p) => {
    p.classList.remove("on");
    p.setAttribute("aria-checked", "false");
  });
  pill.classList.add("on");
  pill.setAttribute("aria-checked", "true");
  strategy = pill.dataset.strategy;
});

/* ---------- RUN / start-light gantry ---------- */
const runBtn = $("run");
runBtn.addEventListener("click", startRun);

/* Plays the red-light sequence, flashes green ("lights out"), then resolves. */
function startLights() {
  return new Promise((resolve) => {
    const g = runBtn;
    g.classList.remove("seq-1", "seq-2", "seq-3", "go");
    const steps = ["seq-1", "seq-2", "seq-3"];
    let i = 0;
    const step = () => {
      if (i < steps.length) {
        g.classList.add(steps[i]);
        i++;
        setTimeout(step, 450);
      } else {
        // lights out — go green
        g.classList.remove("seq-1", "seq-2", "seq-3");
        g.classList.add("go");
        setTimeout(() => {
          g.classList.remove("go");
          resolve();
        }, 500);
      }
    };
    step();
  });
}

async function startRun() {
  if (runBtn.disabled) return;
  // Reflect GSAP availability on the root so CSS only owns the car width
  // transition in the no-GSAP fallback (avoids double-easing). Evaluated
  // here, after defer'd CDN scripts have loaded.
  document.documentElement.classList.toggle("has-gsap", !!window.gsap);
  resetUI();
  runBtn.disabled = true;
  document.body.classList.add("racing");
  $("grid-hint").textContent = "luzes...";

  await startLights();

  $("grid-hint").textContent = "LARGADA!";
  openStream();
}

/* ---------- reset between runs ---------- */
function resetUI() {
  $("lanes").replaceChildren();
  $("timeline").replaceChildren();
  for (const k in lanes) delete lanes[k];
  eventCount = 0;
  updateLogCount();

  for (const id of ["cp-in", "cp-intent", "cp-race", "cp-out"]) {
    const el = $(id);
    el.classList.remove("lit", "blocked");
    const st = el.querySelector(".cp-state");
    st.textContent = st.dataset.default;
  }
  $("in-chips").replaceChildren();
  $("in-masked").textContent = "";
  $("response").textContent = "";
  $("resp-prov").textContent = "";
}

/* ---------- SSE ---------- */
function openStream() {
  const q = encodeURIComponent($("q").value);
  const es = new EventSource(`/viz/stream?q=${q}&strategy=${strategy}`);

  es.onmessage = (msg) => {
    if (msg.data === "[DONE]") {
      es.close();
      finishRun();
      return;
    }
    let e;
    try {
      e = JSON.parse(msg.data);
    } catch (_) {
      return;
    }
    handle(e);
  };
  es.onerror = () => {
    es.close();
    finishRun();
  };
}

function finishRun() {
  runBtn.disabled = false;
  document.body.classList.remove("racing");
  if ($("grid-hint").textContent === "LARGADA!") {
    $("grid-hint").textContent = "corrida encerrada";
  }
}

/* ---------- checkpoints ---------- */
function lightCheckpoint(id, stateText) {
  const el = $(id);
  el.classList.add("lit");
  if (stateText != null) el.querySelector(".cp-state").textContent = stateText;
}

/* ---------- lanes ---------- */
function ensureLane(name) {
  if (lanes[name]) return lanes[name];

  const wrap = document.createElement("div");
  wrap.className = "lane running";

  // Static scaffolding only (no server data) — innerHTML is safe here.
  wrap.innerHTML = `
    <div class="lane-head">
      <span class="lane-name"></span>
      <span class="lane-badge">running</span>
    </div>
    <div class="track">
      <div class="trail"></div>
      <div class="car"><span class="car-glyph">🏎️</span></div>
    </div>
    <div class="lane-tail">
      <span class="telemetry">—<span class="unit"> ms</span></span>
      <span class="sab">
        <button type="button" data-mode="fail"  title="kill">💥</button>
        <button type="button" data-mode="delay" title="+5s">⏱ +5s</button>
        <button type="button" data-mode="clear" title="reset">♻️</button>
      </span>
    </div>`;

  // server-provided name in via textContent ONLY
  wrap.querySelector(".lane-name").textContent = name;

  $("lanes").appendChild(wrap);

  const lane = {
    el: wrap,
    trail: wrap.querySelector(".trail"),
    car: wrap.querySelector(".car"),
    badge: wrap.querySelector(".lane-badge"),
    telem: wrap.querySelector(".telemetry"),
    chunks: 0,
    t0: performance.now(),
  };

  wrap.querySelectorAll(".sab button").forEach((b) => {
    b.addEventListener("click", () => sabotage(name, b.dataset.mode));
  });

  lanes[name] = lane;
  return lane;
}

/* advance a car to pct (0..100). GSAP if present, else CSS transition. */
function moveCar(lane, pct) {
  const target = Math.max(0, Math.min(100, pct));
  lane.trail.style.width = target + "%";
  if (window.gsap) {
    window.gsap.to(lane.car, { width: target + "%", duration: 0.35, ease: "power2.out" });
  } else {
    lane.car.style.width = target + "%"; // CSS transition handles the easing
  }
}

function setTelemetry(lane, ms) {
  lane.telem.replaceChildren();
  lane.telem.append(document.createTextNode(String(ms)));
  const unit = document.createElement("span");
  unit.className = "unit";
  unit.textContent = " ms";
  lane.telem.append(unit);
}

/* ---------- race log ---------- */
function log(t, msg, kind) {
  const li = document.createElement("li");
  if (kind) li.classList.add(kind);
  const ts = document.createElement("span");
  ts.className = "ts";
  ts.textContent = `t=${t == null ? "?" : t}ms`;
  const m = document.createElement("span");
  m.className = "msg";
  m.textContent = msg; // server text via textContent
  li.append(ts, m);
  const tl = $("timeline");
  tl.appendChild(li);
  tl.scrollTop = tl.scrollHeight;
  eventCount++;
  updateLogCount();
}
function updateLogCount() {
  $("log-count").textContent = `${eventCount} evento${eventCount === 1 ? "" : "s"}`;
}

/* ---------- win effects (guarded) ---------- */
function celebrate(lane) {
  if (window.confetti) {
    const r = lane.el.getBoundingClientRect();
    window.confetti({
      particleCount: 90,
      spread: 70,
      startVelocity: 38,
      origin: {
        x: (r.left + r.width * 0.85) / window.innerWidth,
        y: (r.top + r.height / 2) / window.innerHeight,
      },
      colors: ["#f4c20d", "#ffd83b", "#ffffff", "#111111"],
    });
  }
  if (window.gsap) {
    window.gsap.fromTo(lane.el, { scale: 0.99 }, { scale: 1, duration: 0.4, ease: "back.out(2)" });
  }
}

/* ============================================================
   EVENT HANDLER — all 15 types
   ============================================================ */
function handle(e) {
  switch (e.type) {
    // ---- ① Guard In ----
    case "guard_in": {
      lightCheckpoint("cp-in", "PII mascarada");
      const chip = document.createElement("span");
      chip.className = "chip";
      // detail = PII type, content = placeholder; textContent keeps it XSS-safe
      chip.textContent = `${e.detail ?? "?"} → ${e.content ?? ""}`;
      $("in-chips").appendChild(chip);
      log(e.t, `guard_in: ${e.detail ?? ""} → ${e.content ?? ""}`);
      break;
    }
    case "masked_prompt": {
      $("in-masked").textContent = "→ LLM: " + (e.content ?? "");
      lightCheckpoint("cp-in", null);
      break;
    }
    case "blocked": {
      const el = $("cp-in");
      el.classList.remove("lit");
      el.classList.add("blocked");
      el.querySelector(".cp-state").textContent = `BLOQUEADO: ${e.detail ?? ""}`;
      log(e.t, `BLOQUEADO: ${e.detail ?? ""}`, "bad");
      $("grid-hint").textContent = "corrida bloqueada na largada";
      break;
    }

    // ---- ② Intent ----
    case "intent": {
      lightCheckpoint("cp-intent", e.detail ?? "");
      log(e.t, `intent: ${e.detail ?? ""}`);
      break;
    }

    // ---- ③ Race ----
    case "start": {
      lightCheckpoint("cp-race", `estratégia: ${e.detail ?? ""}`);
      log(e.t, `start (${e.detail ?? ""})`, "key");
      // visual launch reinforcement (lights already played on click)
      if (window.gsap) {
        window.gsap.fromTo("#cp-race", { backgroundColor: "rgba(244,194,13,.25)" },
          { backgroundColor: "rgba(244,194,13,.06)", duration: 0.8 });
      }
      break;
    }
    case "decision": {
      log(e.t, `decisão: ${e.detail ?? ""}`);
      break;
    }
    case "provider_start": {
      ensureLane(e.provider);
      log(e.t, `${e.provider} entrou na pista`);
      break;
    }
    case "chunk": {
      const lane = ensureLane(e.provider);
      lane.chunks++;
      moveCar(lane, Math.min(90, lane.chunks * 12));
      setTelemetry(lane, Math.round(performance.now() - lane.t0));
      break;
    }
    case "won": {
      const lane = ensureLane(e.provider);
      lane.el.classList.remove("running", "cancelled", "failed");
      lane.el.classList.add("won");
      lane.badge.textContent = "WON 🏁";
      moveCar(lane, 100);
      setTelemetry(lane, Math.round(performance.now() - lane.t0));
      celebrate(lane);
      log(e.t, `${e.provider} VENCEU`, "win");
      $("grid-hint").textContent = `vencedor: ${e.provider}`;
      break;
    }
    case "cancelled": {
      const lane = ensureLane(e.provider);
      if (!lane.el.classList.contains("won")) {
        lane.el.classList.remove("running");
        lane.el.classList.add("cancelled");
        lane.badge.textContent = "CANCELADO";
      }
      log(e.t, `${e.provider} cancelado (context cancel)`);
      break;
    }
    case "failed": {
      const lane = ensureLane(e.provider);
      lane.el.classList.remove("running");
      lane.el.classList.add("failed");
      lane.badge.textContent = "💥 FALHOU";
      // replace car glyph with explosion, no server data
      const glyph = lane.car.querySelector(".car-glyph");
      if (glyph) glyph.textContent = "💥";
      log(e.t, `${e.provider} falhou: ${e.detail ?? ""}`, "bad");
      break;
    }
    case "done": {
      const lane = ensureLane(e.provider);
      moveCar(lane, 100);
      if (!lane.el.classList.contains("won") &&
          !lane.el.classList.contains("failed") &&
          !lane.el.classList.contains("cancelled")) {
        lane.el.classList.remove("running");
        lane.el.classList.add("done");
        lane.badge.textContent = "FINISH";
      }
      setTelemetry(lane, Math.round(performance.now() - lane.t0));
      log(e.t, `${e.provider} concluiu (100%)`);
      break;
    }

    // ---- ④ Guard Out + streamed answer ----
    case "out_chunk": {
      const rp = $("resp-prov");
      if (!rp.textContent && e.provider) rp.textContent = e.provider;
      // answer is already PII-scrubbed by the proxy; textContent regardless
      $("response").append(document.createTextNode(e.content ?? ""));
      break;
    }
    case "guard_out": {
      const finding = e.content ? `${e.detail ?? ""} → ${e.content}` : (e.detail ?? "limpo");
      lightCheckpoint("cp-out", finding);
      log(e.t, `guard_out: ${finding}`);
      break;
    }

    // ---- global error / DNF ----
    case "error": {
      $("cp-race").classList.add("blocked");
      $("cp-race").querySelector(".cp-state").textContent = "DNF — corrida abortada";
      log(e.t, `DNF — corrida abortada: ${e.detail ?? ""}`, "bad");
      $("grid-hint").textContent = "DNF — corrida abortada";
      break;
    }

    default:
      // unknown type — keep the demo robust, just log it
      log(e.t, `evento: ${e.type}`);
  }
}

/* ---------- sabotage ---------- */
function sabotage(provider, mode) {
  fetch("/viz/sabotage", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ provider, mode, delay_ms: 5000 }),
  }).catch(() => {
    /* sabotage is fire-and-forget; ignore network errors in the demo */
  });
}
