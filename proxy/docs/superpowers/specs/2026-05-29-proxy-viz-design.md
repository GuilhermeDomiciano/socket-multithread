# Visualização Web do LLM Proxy Paralelo — Design Spec

**Data:** 2026-05-29
**Projeto:** Sistemas Paralelos — ULBRA (trabalho final)
**Stack:** Go (stdlib), SSE, HTML + JS puro (zero dependência)
**Base:** estende `docs/superpowers/specs/2026-05-22-llm-proxy-design.md`

---

## Objetivo

Dar uma **visualização web ao vivo** ao proxy LLM existente, tornando o paralelismo visível para uma plateia: três provedores "correndo" em paralelo, o vencedor cancelando os perdedores via `context`, sabotagem ao vivo para provar resiliência, e uma timeline em milissegundos. Serve à apresentação final demonstrando CSP, fan-out e cancelamento por contexto de forma assistível.

**Não-objetivo:** reescrever o backend. Tudo é **aditivo**. O caminho de produção (`/query`, `/v1/chat/completions`) permanece intocado e testado.

---

## Decisões tomadas no brainstorming

| Decisão | Escolha |
|---|---|
| Origem das respostas na demo | **Providers reais** (OpenAI/Anthropic/Gemini com API keys) |
| Estratégias na tela | **Todas as 3**, com seletor (fastest/cheapest/fallback) |
| Timeline | **Ao vivo + persistente** (desenha durante a corrida, fica na tela depois) |
| Arquitetura de telemetria | **Endpoint de viz separado + barramento de eventos (observer)** |
| Sabotagem | **Decorator `Provider`** controlado por endpoint, em runtime |
| Layout | **Pistas horizontais** (estilo drag-race) |
| Frontend | HTML + JS puro, sem framework |

**Princípio mestre:** se `Sink == nil`, o comportamento do router é **idêntico ao atual**. A viz nunca altera o caminho de produção.

---

## Arquitetura

```
                         ┌─────────────────────────────┐
  navegador (corrida) ◄──┤ GET  /viz/stream   (SSE rico)│  NOVO handler
  botões sabotagem   ───►│ POST /viz/sabotage           │  NOVO handler
  página estática    ◄───┤ GET  /            (index.html)│  NOVO (serve static/)
                         └──────────────┬──────────────┘
                                        │ injeta event.Sink no dispatch
                         ┌──────────────▼──────────────┐
                         │ router  (fastest/cheapest/   │  alterado: emite eventos
                         │          fallback)            │  no Sink se presente
                         └──────────────┬──────────────┘
                                        │
                         ┌──────────────▼──────────────┐
                         │ provider.Sabotage (decorator)│  NOVO: envolve provider real
                         │   └─ provider real (OpenAI…) │  injeta delay/erro on-demand
                         └─────────────────────────────┘
```

### Componentes novos

- `event/event.go` — tipo `Event`, interface `Sink`, implementação `ChanSink`.
- `server/handler_viz.go` — handlers `/viz/stream`, `/viz/sabotage`, e estático `/`.
- `provider/sabotage.go` — decorator `Sabotage` + registry.
- `static/index.html` + `static/app.js` — a tela (pistas horizontais).

### Alterações mínimas no existente

- `provider/provider.go` — sem mudança de contrato. (`Sink` é parâmetro de dispatch, não do `Provider`.)
- `router/router.go` e `router/strategy_*.go` — assinaturas passam a aceitar um `event.Sink` opcional; chamam `sink.Emit(...)` nos pontos-chave. Com `Sink == nil`, nenhum `Emit` ocorre.
- `server/server.go` — registrar as 3 rotas novas; `Server` passa a guardar o registry de sabotagem.
- `main.go` — envolver cada provider real num `*provider.Sabotage`, registrar no registry, e injetar o registry no server.

---

## Modelo de eventos (`event/`)

```go
type Event struct {
    Type     string `json:"type"`
    Provider string `json:"provider,omitempty"`
    T        int64  `json:"t"`                 // ms desde o início da corrida
    Content  string `json:"content,omitempty"`
    Detail   string `json:"detail,omitempty"`  // custo calculado, motivo de falha, etc.
}

type Sink interface {
    Emit(Event)
}
```

### `ChanSink`

Encapsula um `chan Event` bufferizado. `Emit` envia no canal com `select` contra um sinal de cancelamento, **nunca bloqueando** se o consumidor sumiu (evita goroutine leak). É concurrency-safe por construção (canal) — várias goroutines de provider emitem em paralelo. Tematicamente coerente: a telemetria flui por channel, como o resto do projeto.

```go
type ChanSink struct {
    ch   chan Event
    done <-chan struct{}
}
func NewChanSink(buf int, done <-chan struct{}) *ChanSink
func (s *ChanSink) Emit(e Event)        // select { case s.ch <- e: case <-s.done: }
func (s *ChanSink) Events() <-chan Event
func (s *ChanSink) Close()              // fecha s.ch (chamado quando o dispatch termina)
```

### Tipos de evento

| Type | Quando | Uso na viz |
|---|---|---|
| `start` | início; `Detail` = estratégia, lista de providers (CSV) | montar as N pistas |
| `provider_start` | goroutine do provider dispara | iniciar a barra |
| `chunk` | provider produz conteúdo (**inclusive perdedores**) | avançar a barra + texto |
| `won` | vencedor eleito (fastest) | destacar vencedor |
| `cancelled` | perdedor recebeu ctx cancel | desbotar barra + "cancelled ❌" |
| `failed` | provider deu erro/timeout; `Detail` = motivo | barra vermelha |
| `decision` | cheapest calculou custos; `Detail` = custos por provider | mostrar raciocínio |
| `done` | vencedor terminou o stream | encerrar |
| `error` | todos falharam; `Detail` = motivo | estado de erro |

### Sequências por estratégia

- **fastest:** `start` → N×`provider_start` → muitos `chunk` (corrida) → `won` + (N−1)×`cancelled` → `done`
- **fallback:** `start` → `provider_start`→`failed` → `provider_start`→`won` → `done` (pistas acendem uma a uma)
- **cheapest:** `start` → `decision` → `provider_start` do escolhido → `chunk` → `done` (se falhar, cai em sequência de fallback)

### Carimbo de tempo

O `T` em ms é calculado pelo **handler** (guarda `startTime` ao iniciar o dispatch; cada `Emit` recebido é carimbado com `time.Since(start).Milliseconds()` na borda, ou o router passa o `T` já calculado a partir de um `start` injetado). **Decisão:** o handler injeta um `func() int64` (relógio relativo) no dispatch, e o router usa-o ao montar eventos. Isso mantém o router testável (relógio mockável) e sem dependência de `time.Now()` direto nos pontos de asserção.

---

## Endpoints novos (`server/handler_viz.go`)

### `GET /viz/stream?q=<prompt>&strategy=<fastest|cheapest|fallback>`

SSE. Fluxo:

1. Valida `q` (não vazio) e `strategy` (uma das três, default `fastest`). Inválido → `400`.
2. Cria `done` (ligado a `r.Context().Done()`), um `ChanSink`, e um relógio relativo.
3. Dispara `router.Dispatch(ctx, req, sink, clock)` numa goroutine; ao terminar, `sink.Close()`.
4. `range` sobre `sink.Events()`: escreve cada evento como `data: <json>\n\n` e dá `Flush()`.
5. Cliente desconecta → `ctx` cancela → providers param → `sink.Emit` deixa de bloquear → goroutine encerra limpa.
6. Ao fechar o canal, escreve `data: [DONE]\n\n`.

EventSource (GET) é suficiente; prompts de demo são curtos. (Anotado como limitação consciente: prompts muito longos esbarram no limite de URL — aceitável para o escopo.)

### `POST /viz/sabotage`

Body JSON:
```json
{ "provider": "openai", "mode": "fail" }
{ "provider": "gemini", "mode": "delay", "delay_ms": 5000 }
{ "provider": "openai", "mode": "clear" }
```
- Resolve `provider` no registry. Inexistente → `404`. `mode` inválido → `400`.
- Ajusta o estado do `*Sabotage` correspondente (thread-safe). Responde `200` com o estado atual.

### `GET /` e estáticos

Serve `static/index.html` e `static/app.js` via `http.FileServer` (ou `embed.FS` para empacotar no binário — **decisão: `embed.FS`**, mantém o "binário único, zero dependência" e evita problemas de path na hora da demo).

---

## Sabotagem (`provider/sabotage.go`)

```go
type Sabotage struct {
    Inner      provider.Provider
    mu         sync.RWMutex
    forceFail  bool
    extraDelay time.Duration
}

func (s *Sabotage) Name() string             { return s.Inner.Name() }
func (s *Sabotage) CostPer1kTokens() float64 { return s.Inner.CostPer1kTokens() }

func (s *Sabotage) Stream(ctx context.Context, req provider.Request, out chan<- provider.Chunk) error {
    s.mu.RLock(); fail, delay := s.forceFail, s.extraDelay; s.mu.RUnlock()
    if fail {
        // emite erro sem chamar o inner; fecha out conforme contrato
    }
    if delay > 0 {
        select { case <-time.After(delay): case <-ctx.Done(): /* aborta */ }
    }
    return s.Inner.Stream(ctx, req, out)
}

func (s *Sabotage) SetFail(bool)            // com lock
func (s *Sabotage) SetDelay(time.Duration)  // com lock
func (s *Sabotage) Clear()                  // zera ambos
```

**Registry:** `map[string]*Sabotage` (nome do provider → decorator), montado no `main.go`. O handler de controle o consulta. Em produção (sem a tela), os decorators ficam inertes → overhead zero.

**Contrato `out`:** o decorator respeita o mesmo contrato do `MockProvider`/providers reais — fecha `out` no fim (sucesso, erro ou cancel) e usa `select` contra `ctx.Done()` ao enviar.

---

## Frontend (`static/`)

**Layout (pistas horizontais — drag race):**
- Topo: input de prompt + seletor de estratégia (3 pills) + botão Run.
- Meio: uma pista por provider (label, barra de progresso que preenche da esquerda pra direita, badge de status WON/cancelled ❌/failed, e controles 💥 derrubar / ⏱ +5s por pista).
- Base: timeline que se preenche ao vivo (`t=Xms · evento`) e persiste após a corrida.

**`app.js` (vanilla):**
- Run → abre `EventSource('/viz/stream?q=...&strategy=...')`.
- `onmessage` → parse do `Event`; atualiza a pista correspondente por `type`:
  - `start`: cria as pistas a partir da lista.
  - `provider_start`: pista em estado "running".
  - `chunk`: incrementa o preenchimento (heurística: por nº de chunks/caracteres recebidos) e acrescenta texto.
  - `won`: pista vencedora destacada.
  - `cancelled`/`failed`: desbota/vermelho + badge.
  - `decision`: exibe custos (cheapest).
  - `done`/`[DONE]`: encerra; timeline permanece.
- Botões de sabotagem → `fetch('/viz/sabotage', {method:'POST', body:...})`.
- Sem build, sem dependências; servido via `embed.FS`.

O preenchimento da barra é **heurístico** (proporcional a chunks recebidos), já que não há total conhecido — o objetivo é mostrar *movimento e ordem de chegada*, não progresso exato.

---

## Tratamento de erros

| Situação | Comportamento |
|---|---|
| `/viz/stream` sem `q` ou strategy inválida | `400` antes de qualquer dispatch |
| Todos os providers falham | evento `error` (estado vermelho na tela); servidor segue de pé |
| Cliente fecha aba | `ctx` cancela dispatch → providers param; `ChanSink.Emit` não bloqueia; goroutine encerra sem leak |
| `/viz/sabotage` provider inexistente | `404` |
| `/viz/sabotage` modo inválido | `400` |
| Provider sabotado com `fail` | emite `failed` na timeline; fastest reelege outro |

Erros nunca viram panic; fluem como eventos (`failed`/`error`) ou status HTTP na borda.

---

## Testes (`go test ./...`, stdlib só)

| Pacote | Cobertura |
|---|---|
| `event/` | `ChanSink.Emit` entrega em ordem; não bloqueia após `done` fechado; `Close` fecha o canal |
| `router/` | cada estratégia emite a **sequência esperada de eventos** num `Sink` mock (fastest → `won` + N−1 `cancelled`; fallback → sequência; cheapest → `decision` + escolhido). **Testes atuais seguem passando com `Sink == nil`.** Relógio relativo mockado para asserções determinísticas. |
| `provider/sabotage` | `fail` emite `failed` sem chamar o inner; `delay` respeita `ctx` (cancela durante o sleep); `clear` restaura; passa o check de interface `Provider` |
| `server/handler_viz` | `httptest`: `/viz/stream` produz SSE com sequência de eventos esperada (usando MockProviders); `/viz/sabotage` altera estado e responde 200/404/400; estáticos servidos |

**Fora do `go test`:** `static/` (JS) — validação manual na demo. Race condition proposital — arquivo/exemplo à parte com `go run -race`, segmento didático, **não** entra no código de produção nem no MVP.

---

## Escopo do MVP (ordem de prioridade)

1. **Núcleo:** `event/` + router emitindo eventos + `/viz/stream` + frontend das pistas (fastest). → "a corrida".
2. **Clímax:** `provider/sabotage` + `/viz/sabotage` + botões na tela. → re-roteamento ao vivo.
3. **Fechamento:** timeline ao vivo/persistente (já vem dos eventos com `T`).
4. **Estratégias extras:** seletor cheapest/fallback + eventos `decision`/sequência.

Fora do MVP (bônus): raio-X de goroutines, economia acumulada em $, health-check, modo comparação lado a lado.

---

## Módulo Go

Permanece `module github.com/domiciano/llm-proxy`, **sem dependências externas**. `embed` é stdlib. Mantém o argumento didático: Go resolve com primitivas nativas.
