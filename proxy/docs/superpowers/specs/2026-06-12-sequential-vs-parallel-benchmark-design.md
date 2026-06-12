# Sequencial vs Paralelo — benchmark medido ao vivo + rework do dashboard

**Data:** 2026-06-12
**Status:** Aprovado (brainstorming)
**Escopo:** Backend Go (novo modo de benchmark medido) **+** rework do frontend Parallel GP
(mantendo o visual atual), pra a apresentação de sistemas paralelos de hoje.

## Problema

O dashboard Parallel GP ficou bonito, mas na hora de apresentar ele **não tem valor**:
1. **Não prova o conceito** — a corrida é vistosa, mas não deixa óbvio o *ganho* de paralelismo;
   falta o NÚMERO que fecha o argumento da aula ("paralelo foi Nx mais rápido que sequencial").
2. **Parece brinquedo** — funciona, mas não passa a sensação de resolver um problema real.
3. **Não conta história** — tem os componentes (guard, intent, corrida, scrub) mas não tem um
   roteiro de 5 minutos que amarre tudo num "porquê" convincente.

Além disso, o frontend "não mostra bem as coisas" no projetor — legibilidade e clareza de estado
precisam melhorar, mantendo a identidade Racing/GP.

## Solução

Um **benchmark A/B medido de verdade**: o mesmo prompt roda em dois modos *gather-all* (buscar a
resposta de **todos** os providers) e a UI cronometra os dois lado a lado.

- **Sequencial:** chama os providers um após o outro, cada um até o fim → tempo ≈ **soma** das latências.
- **Paralelo:** dispara todos concorrentemente, espera **todos** → tempo ≈ **o mais lento** (máximo).
- **Speedup = soma / máximo ≈ N×** — o resultado canônico de "embarrassingly parallel", robusto e
  sempre impressionante.

Isso mata os três gaps de uma vez: prova o conceito (número), deixa de ser brinquedo (problema real:
"preciso consultar/comparar N providers — paralelizar reduz o tempo de soma para máximo"), e vira
história (sequencial lento → paralelo rápido → 💥 mato um provider → o sequencial incha, o paralelo
aguenta).

A corrida *race-to-first* atual (`fastest`, cancela perdedores) **permanece** como beat de
acompanhamento ("dá pra ser ainda mais esperto: corre e cancela os perdedores").

## Backend (Go)

Mudanças contidas; as funções novas são **mais simples** que `Fastest` (sem lógica de vencedor/cancel),
o que facilita manter `go test -race ./...` limpo.

### `event.Event` — campo `Phase`

Adicionar um campo:

```go
type Event struct {
	Type     string `json:"type"`
	Provider string `json:"provider,omitempty"`
	T        int64  `json:"t"`
	Content  string `json:"content,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Phase    string `json:"phase,omitempty"` // "seq" | "par" durante o benchmark; vazio nos demais modos
}
```

`omitempty` ⇒ todos os eventos existentes continuam idênticos no wire (zero mudança de comportamento).

### `router.SequentialAll(ctx, providers, req, sink) <-chan provider.Chunk`

Novo arquivo `router/strategy_sequential.go`. Roda **cada** provider, um após o outro, **até o fim**
(gather-all). Diferente de `Fallback`, **não para no primeiro sucesso**: um provider que falha apenas
contribui com seu tempo-até-falhar e seguimos para o próximo. Emite `provider_start`/`chunk`/`done`/
`failed` por provider e encaminha todos os chunks em `out`. Fecha `out` ao terminar todos.

### `router.ParallelAll(ctx, providers, req, sink) <-chan provider.Chunk`

Novo arquivo `router/strategy_parallel_all.go`. Dispara **todos** concorrentemente e espera **todos**
terminarem (**sem** cancelamento — é gather-all, não race-to-first). Mesma família de eventos por
provider, chunks encaminhados (interleaved). Fecha `out` quando todos terminam. Segue os invariantes
de concorrência do projeto: cada `Provider.Stream` fecha seu `out` em todos os caminhos; `ParallelAll`
usa `sync.WaitGroup` + canal merge (como `Fastest`, porém sem winner/cancel).

### `phaseSink` — decorator de fase

Em `pipeline` (ou `event`), um Sink fino que carimba a fase e repassa:

```go
type phaseSink struct {
	inner event.Sink
	phase string
}
func (p phaseSink) Emit(e event.Event) { e.Phase = p.phase; p.inner.Emit(e) }
```

Assim `SequentialAll`/`ParallelAll` permanecem **agnósticas de fase**.

### `Gateway.Benchmark(ctx, prompt, sink) error`

Novo método em `pipeline`. Orquestra o A/B reaproveitando o guard de entrada:

1. `in := g.Input.Inspect(prompt)`; emite `guard_in` para cada finding de PII (igual `Process`).
2. Se `in.Blocked` → emite `blocked`, retorna erro embrulhando `ErrBlocked` (benchmark não roda).
3. `emit(masked_prompt)`; monta `req` com `in.Text`.
4. **Fase seq:** `t0 := time.Now()`; drena `SequentialAll(ctx, providers, req, phaseSink{sink,"seq"})`
   até fechar; `seqMs := time.Since(t0).Milliseconds()`.
5. **Fase par:** `t1 := time.Now()`; drena `ParallelAll(ctx, providers, req, phaseSink{sink,"par"})`
   até fechar; `parMs := time.Since(t1).Milliseconds()`.
6. Emite `speedup` (ver contrato). Sem intent (forçamos os dois modos) e sem scrub (no benchmark o
   número é o protagonista, não a resposta).

`time.Now()` é Go de produção normal (o stamp relativo do `ChanSink` continua valendo para `T`).

### `/viz/stream?strategy=benchmark`

`handleVizStream` ganha `benchmark` na lista de estratégias válidas e, quando for o caso, chama
`s.Gateway.Benchmark(ctx, q, sink)` em vez de `Process`. Mesmo encerramento por `[DONE]`; o tratamento
de `ErrBlocked` é idêntico ao atual. `POST /viz/sabotage` é reaproveitado sem mudança.

### Testes Go

`SequentialAll`, `ParallelAll` e `Benchmark` testados com `MockProvider` sob `-race`: ordem/contagem
de eventos, fechamento de canais, fase carimbada, e o caso "um provider falha" (sequencial continua;
paralelo não cancela). Backend de produção (`handleQuery`) intocado.

## Contrato de eventos (delta)

Tudo que já existe continua; o benchmark só **acrescenta** a fase e um evento novo.

| type | phase | payload | uso na UI |
|------|-------|---------|-----------|
| `guard_in` / `masked_prompt` / `blocked` | — | como hoje | ① Guard In |
| `provider_start` | `seq`\|`par` | `provider` | cria a barra do provider na trilha da fase |
| `chunk` | `seq`\|`par` | `provider`, `content` | avança a barra do provider na fase |
| `done` | `seq`\|`par` | `provider` | fixa a barra cheia |
| `failed` | `seq`\|`par` | `provider`, `detail`=erro | barra vermelha (💥) — incha a soma sequencial |
| `speedup` | — | `Content`=`{"seq_ms":1900,"par_ms":620,"factor":3.06}`; `Detail`=linha PT p/ o log | leitura gigante `N× MAIS RÁPIDO` |

Stream encerra com `data: [DONE]`.

## Frontend — rework mantendo o visual Racing/GP

Implementado na execução pela skill **frontend-design**, **preservando a identidade visual atual**
(asfalto escuro, amarelo-corrida, tipografia condensada + monoespaçada, bandeira/semáforo). Os três
arquivos (`index.html`, `styles.css`, `app.js`) são **revisados** — não jogados fora —, porque os
novos eventos/fases exigem mexer no render e é a chance de melhorar a legibilidade no projetor.

**Objetivos do rework (a reclamação real: "não mostra bem as coisas"):**

1. **Visão de benchmark (herói novo).** Nova pill `benchmark`. Ao rodar, o herói vira **duas trilhas
   empilhadas** — `SEQUENCIAL` e `PARALELO` — cada uma com as barras dos providers e um **cronômetro**.
   Na fase seq as barras aparecem **em sequência** (cronômetro somando); na fase par, **sobrepostas**
   (cronômetro = o mais lento). No evento `speedup`, leitura gigante **`3.1× MAIS RÁPIDO`** com flash
   (confetti se disponível). Eventos roteados para a trilha certa por `phase`.
2. **Legibilidade de projetor.** Tipos e números maiores, contraste de estado mais forte
   (correndo/vencedor/cancelado/falha), rótulos sempre visíveis sem hover; o que importa numa sala
   grande lido de longe.
3. **Clareza de estado da pista normal.** A corrida `fastest` (race-to-first) continua, com
   vencedor/cancelado/falha mais explícitos — sem perder o "cancel-losers" como showpiece.
4. **Degradação graciosa preservada.** GSAP + canvas-confetti via CDN **com fallback CSS obrigatório**
   (sem internet a demo não quebra). Render **XSS-safe**: strings do servidor só via `textContent`.

A trilha do pipeline (① Guard In · ② Intent · ③ Race · ④ Guard Out) e o race log recolhível
permanecem; ② Intent fica oculto/neutro no modo benchmark (não há classificação).

## Casos de borda

- **Prompt bloqueado:** `blocked` → ① vermelho, benchmark não roda; ②③④ apagados.
- **Todos os providers falham:** mostra os dois tempos mesmo assim (tempo-até-falhar é dado) com nota
  "todos falharam"; `factor` exibido com cautela (sem divisão por zero — se `parMs==0`, mostrar "—").
- **Provider sabotado (💥):** barra vermelha nas duas trilhas; **infla a soma sequencial** (gancho de
  resiliência) e o paralelo absorve melhor.
- **CDN fora:** fallback CSS.
- **Cliente desconecta:** já tratado server-side (`ChanSink` não vaza goroutines); a UI fecha o
  `EventSource` em `[DONE]`/`onerror`.

## Mapa para o ROTEIRO de apresentação

| Beat | Momento visual |
|------|----------------|
| ganho de paralelismo (o número) | benchmark: `Sequencial` somando vs `Paralelo` no máximo → `N× MAIS RÁPIDO` |
| problema real (não-brinquedo) | "preciso de N providers; paralelizar troca soma por máximo" |
| resiliência | 💥 num provider → incha a soma sequencial, paralelo aguenta |
| fan-out / first-response-wins / context-cancel | corrida `fastest` (mantida) com vencedor + cancelados |
| guardrails | ① e ④ acendendo; chips de PII; `blocked` vermelho |
| intent routing | ② com estratégia escolhida (modos não-benchmark) |

## Verificação

Sem harness de teste de frontend (não vamos criar sob deadline).

1. `./run.sh` mantém `gofmt`/`vet`/`build` + `go test -race ./...` verdes (inclui os testes novos de
   `SequentialAll`/`ParallelAll`/`Benchmark`).
2. Dirigir no navegador: rodar `benchmark` e ver as duas trilhas + `N×`; rodar `fastest` e ver
   largada→bandeira→cancelado; 💥 num provider e ver a soma sequencial inchar; PII e injection nos
   guards.
3. Simular CDN fora (bloquear scripts) e confirmar o fallback CSS.

## Fora de escopo (YAGNI)

- Mudar `handleQuery`/endpoints de produção, providers, ou as estratégias existentes
  (`fastest`/`fallback`/`cheapest`).
- Mostrar a resposta scrubbed durante o benchmark (o número é o protagonista).
- Contrafactual calculado no frontend (queremos medido de verdade).
- Build step / bundler / framework; persistência; testes automatizados de frontend.
