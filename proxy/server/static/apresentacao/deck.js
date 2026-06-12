/* Race Briefing — navegação, HUD, reveals e PIT STOP. Vanilla, sem deps. */
(() => {
  // reveals só são escondidos quando há JS para revelá-los de volta
  document.body.classList.add('js');

  const laps = Array.from(document.querySelectorAll('.lap'));
  const fill = document.querySelector('.hud-fill');
  const lapNo = document.querySelector('.hud-lap');
  const overlay = document.getElementById('grid-overlay');
  const pad = (n) => String(n).padStart(2, '0');

  let current = 0;

  // Seção ativa → HUD + dispara animações de diagrama (.seen)
  const activeObs = new IntersectionObserver((entries) => {
    for (const e of entries) {
      if (!e.isIntersecting) continue;
      current = laps.indexOf(e.target);
      lapNo.textContent = `LAP ${pad(current + 1)}/${pad(laps.length)}`;
      fill.style.width = `${((current + 1) / laps.length) * 100}%`;
      e.target.classList.add('seen');
    }
  }, { threshold: 0.5 });
  laps.forEach((s) => activeObs.observe(s));

  // Reveal com stagger (uma vez só por elemento)
  const revealObs = new IntersectionObserver((entries) => {
    for (const e of entries) {
      if (!e.isIntersecting) continue;
      e.target.classList.add('in');
      revealObs.unobserve(e.target);
    }
  }, { threshold: 0.25 });
  document.querySelectorAll('.rv').forEach((el) => revealObs.observe(el));

  // Navegação por teclado
  const go = (i) => laps[Math.max(0, Math.min(laps.length - 1, i))]
    .scrollIntoView({ behavior: 'smooth' });
  document.addEventListener('keydown', (ev) => {
    if (ev.target.matches('input, textarea')) return;
    switch (ev.key) {
      case 'ArrowRight': case 'PageDown': case ' ':
        ev.preventDefault(); go(current + 1); break;
      case 'ArrowLeft': case 'PageUp':
        ev.preventDefault(); go(current - 1); break;
      case 'Home': ev.preventDefault(); go(0); break;
      case 'End': ev.preventDefault(); go(laps.length - 1); break;
      case 'g': case 'G': overlay.hidden = !overlay.hidden; break;
      case 'Escape': overlay.hidden = true; break;
    }
  });
  lapNo.addEventListener('click', () => { overlay.hidden = !overlay.hidden; });
  overlay.addEventListener('click', (ev) => {
    if (ev.target.closest('a[href^="#lap-"]') || ev.target === overlay) overlay.hidden = true;
  });

  // PIT STOP — guardrails ao vivo via POST /query
  const input = document.getElementById('pit-input');
  const send = document.getElementById('pit-send');
  const out = document.getElementById('pit-out');
  const prov = document.getElementById('pit-provider');
  if (!input || !send || !out) return;

  document.querySelectorAll('[data-preset]').forEach((btn) =>
    btn.addEventListener('click', () => { input.value = btn.dataset.preset; input.focus(); }));

  async function run() {
    const prompt = input.value.trim();
    if (!prompt) return;
    out.dataset.state = 'running';
    out.textContent = '… na pista';
    prov.textContent = '—';
    try {
      const res = await fetch('/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: [{ role: 'user', content: prompt }] }),
      });
      if (res.status === 403) {
        const body = await res.json();
        out.dataset.state = 'blocked';
        out.textContent = `HTTP 403 — ${body.error}\n${body.reason || ''}`;
        return;
      }
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      out.dataset.state = 'ok';
      out.textContent = '';
      const reader = res.body.getReader();
      const dec = new TextDecoder();
      let buf = '';
      for (;;) {
        const { done, value } = await reader.read();
        if (done) return;
        buf += dec.decode(value, { stream: true });
        let nl;
        while ((nl = buf.indexOf('\n\n')) >= 0) {
          const line = buf.slice(0, nl).trim();
          buf = buf.slice(nl + 2);
          if (!line.startsWith('data: ')) continue;
          const data = line.slice(6);
          if (data === '[DONE]') return;
          const p = JSON.parse(data);
          if (p.error) {
            out.dataset.state = 'error';
            out.textContent += `\n[erro] ${p.error}`;
            return;
          }
          if (p.provider) prov.textContent = p.provider;
          out.textContent += p.content || '';
        }
      }
    } catch (err) {
      out.dataset.state = 'error';
      out.textContent = `sem conexão com o gateway — ${err.message}`;
    }
  }
  send.addEventListener('click', run);
  input.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') run(); });
})();
