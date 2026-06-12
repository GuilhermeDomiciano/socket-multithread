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

  // Seção ativa → HUD + dispara animações de diagrama (.seen).
  // rootMargin de -50% marca ativa a seção que cruza o CENTRO do viewport:
  // threshold 0.5 nunca dispararia para seções mais altas que 2× o viewport
  // (zoom alto no projetor), travando HUD e navegação por teclado.
  const activeObs = new IntersectionObserver((entries) => {
    for (const e of entries) {
      if (!e.isIntersecting) continue;
      current = laps.indexOf(e.target);
      lapNo.textContent = `LAP ${pad(current + 1)}/${pad(laps.length)}`;
      fill.style.width = `${((current + 1) / laps.length) * 100}%`;
      e.target.classList.add('seen');
    }
  }, { rootMargin: '-50% 0px -50% 0px', threshold: 0 });
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

  // Navegação por teclado. current é atualizado aqui (otimista) e não só
  // pelo observer — teclas em sequência rápida calculariam do lap errado.
  const go = (i) => {
    current = Math.max(0, Math.min(laps.length - 1, i));
    laps[current].scrollIntoView({ behavior: 'smooth' });
  };
  document.addEventListener('keydown', (ev) => {
    if (ev.metaKey || ev.ctrlKey || ev.altKey) return; // Cmd+← = Back etc.
    if (ev.target.matches('input, textarea')) return;
    if (!overlay.hidden && ev.key !== 'Escape' && ev.key !== 'g' && ev.key !== 'G') return;
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

  const show = (text) => { // append mantendo o fim visível (max-height + overflow)
    out.textContent += text;
    out.scrollTop = out.scrollHeight;
  };

  let running = false; // re-entrância: duplo-Enter embaralharia dois streams
  async function run() {
    const prompt = input.value.trim();
    if (!prompt || running) return;
    running = true;
    send.disabled = true;
    out.dataset.state = 'running';
    out.textContent = '… na pista';
    prov.textContent = '—';
    try {
      const res = await fetch('/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ messages: [{ role: 'user', content: prompt }] }),
        signal: AbortSignal.timeout(25000), // abaixo do timeout de 30s do servidor
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
        buf += dec.decode(value, { stream: true }).replace(/\r\n/g, '\n');
        let nl;
        while ((nl = buf.indexOf('\n\n')) >= 0) {
          const line = buf.slice(0, nl).trim();
          buf = buf.slice(nl + 2);
          if (!line.startsWith('data: ')) continue;
          const data = line.slice(6);
          if (data === '[DONE]') return;
          let p;
          try { p = JSON.parse(data); } catch { continue; } // evento malformado ≠ queda de conexão
          if (p.error) {
            out.dataset.state = 'error';
            show(`\n[erro] ${p.error}`);
            return;
          }
          if (p.provider) prov.textContent = p.provider;
          show(p.content || '');
        }
      }
    } catch (err) {
      const timedOut = err.name === 'TimeoutError' || err.name === 'AbortError';
      if (out.dataset.state === 'ok' && out.textContent) {
        show(`\n[stream interrompido — ${timedOut ? 'timeout' : err.message}]`);
      } else {
        out.dataset.state = 'error';
        out.textContent = timedOut
          ? 'tempo esgotado — o gateway não respondeu'
          : `sem conexão com o gateway — ${err.message}`;
      }
    } finally {
      running = false;
      send.disabled = false;
    }
  }
  send.addEventListener('click', run);
  input.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') run(); });
})();
