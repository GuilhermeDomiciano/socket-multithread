# Parallel GP — Redesign completo do dashboard web

**Data:** 2026-06-12
**Status:** Aprovado (brainstorming)
**Escopo:** Frontend-only. Substitui inteiramente `server/static/index.html` e `server/static/app.js`. **Zero mudança no backend Go.**

## Contexto e objetivo

O dashboard atual (um `index.html` escuro + `app.js` vanilla) funciona mas é visualmente fraco para apresentar o trabalho na aula de *sistemas paralelos*. Vamos jogá-lo fora e reconstruir do zero com tema **Racing / GP**, deixando **visceral** o conceito central — *fan-out → first-response-wins → context cancel* — sem perder a prova de que o sistema é completo (guardrails, intent, scrub).

O contrato de dados já existe e **não muda**:

- `GET /viz/stream?q=<prompt>&strategy=<auto|fastest|cheapest|fallback>` → SSE de eventos JSON.
- `POST /viz/sabotage` body `{provider, mode: "fail"|"delay"|"clear", delay_ms}`.

### Eventos consumidos (do `event.Event`)

Campos: `type`, `provider`, `t` (ms desde o início), `content`, `detail`.

| type | origem | uso na UI |
|------|--------|-----------|
| `guard_in` | pipeline | acende ① Guard In; mostra chip de PII mascarada (`detail`=tipo, `content`=placeholder) |
| `masked_prompt` | pipeline | mostra o prompt que de fato vai ao LLM em ① |
| `blocked` | pipeline | ① fica vermelho (`detail`=motivo); corrida não larga |
| `intent` | pipeline | acende ② Intent com o motivo (`detail`) |
| `start` | router | acende ③ Race; arma o semáforo (`detail`=estratégia) |
| `decision` | cheapest | linha extra no race log (ordenação por custo) |
| `provider_start` | router | cria a pista do provider |
| `chunk` | router | avança o carro da pista (`provider`) |
| `won` | fastest | bandeira quadriculada + WON na pista vencedora |
| `cancelled` | fastest | estampa CANCELADO + carro para/desbota |
| `failed` | router | pista vermelha + 💥 (`detail`=erro) |
| `done` | router | finaliza a pista (100%) |
| `out_chunk` | scrub | streama a resposta da IA (`provider`, `content`) — já PII-scrubbed |
| `guard_out` | scrub | acende ④ Guard Out (`detail`=tipo, `content`=placeholder) |
| `error` | router/pipeline | estado DNF / corrida abortada (`detail`) |

Stream encerra com `data: [DONE]`.

## Direção visual

Tema **Racing / GP**: asfalto escuro, amarelo-corrida (`#f4c20d`) como acento, bandeira quadriculada no vencedor, números de telemetria em monoespaçado. Tudo grande e legível no projetor. Acentos de estado: vencedor dourado/checkered, cancelado cinza-fantasma, falha vermelho.

## Anatomia da tela (topo → base)

1. **Linha de largada (control bar)**
   - Input de prompt (valor inicial de exemplo).
   - Pills de estratégia: `auto` (default) · `fastest` · `cheapest` · `fallback`.
   - Botão **RUN** estilizado como **semáforo de largada**: 3 luzes vermelhas → verde no clique, então dispara o `EventSource`.

2. **Trilha do pipeline (4 checkpoints)**
   Acendem em sequência conforme os eventos: `① Guard In` → `② Intent` → `③ Race` → `④ Guard Out`.
   - ① mostra chips de PII mascarada (de `guard_in`) e o prompt final (`masked_prompt`); vira vermelho em `blocked`.
   - ② mostra estratégia + motivo (`intent`).
   - ③ é o estado da corrida (correndo/encerrada).
   - ④ mostra achados do scrub (`guard_out`).

3. **A PISTA (herói visual, centro)**
   - Uma pista asfaltada por provider (criada em `provider_start`), com marcações de pista.
   - Um **carro** avança esquerda→direita a cada `chunk` do provider; latência em ms (de `t`) exibida na pista.
   - `won`: bandeira quadriculada + badge **WON** + brilho dourado na pista vencedora.
   - `cancelled`: estampa **CANCELADO**, carro para e desbota — visualiza o *context cancel*.
   - `failed`: pista vermelha + ícone de batida 💥 + motivo.
   - `done`: pista a 100%.
   - **Controles de sabotagem por pista:** `💥 matar` · `⏱ +5s` · `♻️ reset` → `POST /viz/sabotage`.

4. **Resposta da IA**
   - Streaming dos `out_chunk`, rotulada com o provider vencedor. Conteúdo já PII-scrubbed (usar `textContent`, nunca `innerHTML`).

5. **Race log (recolhível)**
   - Ticker cronológico de eventos com timestamps em ms, para a hora de "mostrar a mecânica".

## Arquitetura do frontend

Três arquivos servidos diretamente pelo handler estático do Go (sem build):

- `server/static/index.html` — estrutura semântica + slots para pipeline, pista, resposta, log; tags de CDN.
- `server/static/styles.css` — tema GP completo (asfalto, amarelo, bandeira, estados).
- `server/static/app.js` — conexão SSE (`EventSource`), máquina de render por `type` de evento, animações, sabotagem.

### Bibliotecas via CDN (com fallback obrigatório)

- **GSAP** — movimento suave dos carros e bandeira.
- **canvas-confetti** — flash no momento da vitória (flair).

**Degradação graciosa:** se o CDN não carregar (sala sem internet), `app.js` detecta a ausência de `window.gsap` / `confetti` e cai para **transições CSS puras** (como hoje, via `style.width` + `transition`). A demo **nunca** quebra por causa de CDN. Nenhuma lib é obrigatória para a funcionalidade.

### Segurança de render

Strings vindas do servidor (nomes de provider, `detail`, conteúdo) só entram no DOM via `textContent` ou após escape (`esc()`), nunca por `innerHTML` com interpolação — mantém o XSS-safe que o `app.js` atual já praticava.

## Casos de borda

- **Todos os providers falham** (`error` / N×`failed`): estado **"DNF — corrida abortada"** em vermelho; nenhuma bandeira.
- **Prompt bloqueado** (`blocked`): ① Guard In vermelho com motivo; a corrida não larga; ②③④ permanecem apagados.
- **CDN indisponível:** fallback CSS (acima).
- **Cliente desconecta:** já tratado server-side (o `ChanSink` não vaza goroutines); a UI só fecha o `EventSource` em `[DONE]` / `onerror`.
- **Estratégia `fallback`/`cheapest`:** uma pista de cada vez (não há corrida simultânea); a UI mostra as pistas surgindo em sequência e o `decision`/`failed`→próximo provider no log.

## Mapa para o ROTEIRO de apresentação

| Beat do ROTEIRO | Momento visual |
|---|---|
| fan-out | largada simultânea de todas as pistas em `t=0` |
| first-response-wins | bandeira quadriculada na primeira pista a responder |
| context cancel | estampa CANCELADO nas demais no mesmo instante |
| resiliência / re-roteamento | botão 💥 mata um provider → outro assume |
| guardrails | checkpoints ① e ④ acendendo; chips de PII; `blocked` em vermelho |
| intent routing | checkpoint ② com estratégia escolhida + motivo |

## Verificação

O projeto não tem harness de teste de frontend e não vamos criar um sob deadline. Validação:

1. `./run.sh` (mantém `gofmt`/`vet`/`build` + `go test -race ./...` verdes — backend intocado).
2. Dirigir o dashboard no navegador: rodar `auto`/`fastest` e ver largada→bandeira→cancelado; clicar 💥 numa pista e ver o re-roteamento; enviar prompt com PII e ver os chips em ① e ④; enviar prompt de injection e ver `blocked`.
3. Simular CDN fora (bloquear scripts) e confirmar o fallback CSS.

## Fora de escopo (YAGNI)

- Qualquer mudança no Go (handlers, eventos, rotas, providers).
- Build step / bundler / framework.
- Persistência de histórico de corridas, multi-prompt simultâneo, autenticação.
- Testes automatizados de frontend.
