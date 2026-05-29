# Smart Gateway — Guardrails + Roteamento por Intenção — Design Spec

**Data:** 2026-05-29
**Projeto:** Sistemas Paralelos — ULBRA (trabalho final)
**Stack:** Go (stdlib), SSE, HTML + JS puro (zero dependência)
**Base:** estende o LLM Proxy Paralelo (`2026-05-22-llm-proxy-design.md`) e a visualização web (`2026-05-29-proxy-viz-design.md`)

---

## Objetivo

Transformar o proxy paralelo num **Smart LLM Gateway** didático, adicionando um **pipeline** ao redor do roteamento paralelo já existente:

```
Cliente → [Guardrail Input] → [Intent → estratégia] → [Router paralelo] → [Guardrail Output] → Cliente
```

- **Guardrail Input:** mascara PII (regex) e bloqueia injeção de prompt (HTTP 403).
- **Intent:** classifica o prompt por heurística e escolhe a estratégia de roteamento (simples→`cheapest`, complexo→`fastest`).
- **Router:** o fan-out paralelo / cancelamento por contexto que já existe.
- **Guardrail Output:** higieniza PII que vaze na resposta, durante o streaming.

Tudo em **Go puro / stdlib**, reaproveitando o `event.Sink` e o dashboard. O ponto pedagógico: casar o **padrão pipeline/middleware** com o **paralelismo** ("só paraleliza quando o prompt justifica").

**Inspiração:** porta as ideias de um Smart LLM Gateway em Python (FastAPI + Presidio + semantic-router). Decisão consciente: reimplementar em Go/regex/heurística em vez de ML, mantendo a tese do projeto (um binário, zero dependência, paralelismo nativo).

**Não-objetivo:** Presidio/NER, embeddings/semantic-router, cache, telemetria, dashboard admin editável — ficam para specs futuros. O endpoint `/v1/chat/completions` permanece direto no router neste spec (guardrails nele = follow-up).

---

## Decisões tomadas no brainstorming

| Decisão | Escolha |
|---|---|
| Stack | **Go puro** (regex p/ PII, heurística p/ intenção) — sem ML/Python |
| Escopo do 1º spec | **Guardrails bidirecionais + roteamento por intenção** |
| Política dos guardrails | **PII mascara; injeção bloqueia (403)** |
| Intenção → roteamento | **Intenção escolhe a estratégia** (`cheapest`/`fastest`) |
| Arquitetura | Estágios de pipeline em pacotes próprios (`guardrail`, `intent`, `pipeline`); orquestração no `pipeline` |
| Output guard no stream | Buffer de borda (segura `K` bytes); trade-off documentado |

**Princípio mestre:** o núcleo paralelo (`router`, `event`, `stream`) fica intacto. As novas peças são aditivas e cada uma é testável isoladamente.

---

## Pacotes novos

### `guardrail/`

```go
type Finding struct {
    Type        string // "email" | "cpf" | "phone" | "credit_card" | "injection"
    Placeholder string // ex: "[REDACTED_EMAIL_0]" (vazio para injeção)
    Count       int
}

type Result struct {
    Text     string    // texto possivelmente mascarado
    Findings []Finding
    Blocked  bool
    Reason   string
}

type Guard interface {
    Inspect(text string) Result
}
```

- **`PIIGuard`** — regex compiladas; **mascara**, nunca bloqueia. Substitui cada ocorrência por `[REDACTED_<TIPO>_<n>]` com `n` sequencial por tipo. Tipos (foco pt-BR):

  | Tipo | Regex |
  |---|---|
  | email | `[\w.+-]+@[\w-]+\.[\w.-]+` |
  | cpf | `\d{3}\.?\d{3}\.?\d{3}-?\d{2}` |
  | phone | `(?:\+?55\s?)?\(?\d{2}\)?\s?9?\d{4}-?\d{4}` |
  | credit_card | `\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}` |

  A ordem de aplicação é: credit_card → cpf → phone → email (mais específico/longo primeiro, para evitar que a regex de telefone "coma" pedaços de cartão). Cada `Finding` carrega `Type`, `Placeholder` do primeiro match e `Count`.

- **`InjectionGuard`** — padrões (case-insensitive, pt+en); se algum casa → `Blocked=true`, `Reason` com o padrão; não altera o texto. Padrões iniciais: `ignore (as )?instruç(ões|oes) anteriores`, `ignore (all )?previous instructions`, `esqueça (as )?instruções`, `disregard (the )?(above|previous)`, `reveal your system prompt`, `mostre (o )?seu (system )?prompt`, `you are now`, `aja como`, `jailbreak`, `\bDAN\b`.

- **`Chain []Guard`** — implementa `Guard`; aplica em ordem, threading o `Text`, acumulando `Findings`; **para no primeiro `Blocked`** (retorna imediatamente com `Blocked=true`).

### `intent/`

```go
type Intent struct {
    Class    string          // "simple" | "complex"
    Strategy router.Strategy  // "cheapest" | "fastest"
    Reason   string
}

func Classify(prompt string) Intent
```

Heurística (ordem):
1. keywords de profundidade (`explique`, `detalhe`, `analise`, `compare`, `passo a passo`, `código`, `implemente`, `disserte`, `história`, `profundidade`) → `complex` / `fastest`.
2. keywords transacionais (`oi`, `olá`, `traduza`, `resuma`, `defina`, `qual`, `quanto`, saudações) → `simple` / `cheapest`.
3. tiebreaker por tamanho: `len(prompt) > 200` → `complex`/`fastest`; senão `simple`/`cheapest`.

`Reason` é uma frase legível (ex: `complex: keyword 'explique' (+412 chars) → fastest`). Comparação case-insensitive. `intent` importa `router` apenas para o tipo `Strategy` (sem ciclo: `router` não importa `intent`).

### `pipeline/`

```go
type Gateway struct {
    Input  guardrail.Guard // Chain{InjectionGuard, PIIGuard}
    Output guardrail.Guard // PIIGuard
    Router *router.Router
}

// ErrBlocked é retornado quando o Input guard bloqueia (injeção).
var ErrBlocked = errors.New("request blocked by guardrail")

// Process roda o pipeline completo e devolve um canal de chunks já higienizados.
// strategyOverride: "" ou "auto" → usa o classificador; senão usa a estratégia dada.
func (g *Gateway) Process(ctx context.Context, prompt string, strategyOverride router.Strategy, sink event.Sink) (<-chan provider.Chunk, error)
```

Fluxo de `Process`:
1. `inRes := g.Input.Inspect(prompt)`. Para cada `Finding`, emite `guard_in` (`Detail` = `"<tipo>×<count>"`, `Content` = placeholder). Emite também um evento `masked_prompt` (`Content` = `inRes.Text`) para o dashboard mostrar o que foi ao LLM.
2. Se `inRes.Blocked`: emite `blocked` (`Detail` = `inRes.Reason`) e retorna `nil, ErrBlocked`.
3. Estratégia: se `strategyOverride != "" && != "auto"`, usa-a; senão `it := intent.Classify(inRes.Text)`, emite `intent` (`Detail` = `it.Reason`), usa `it.Strategy`.
4. `rtr := &router.Router{Providers: g.Router.Providers, Strategy: estrategia}`; `chunks, err := rtr.Dispatch(ctx, provider.Request{Messages: [{user, inRes.Text}]}, sink)`. Se `err != nil`, repassa.
5. Retorna `scrub(ctx, chunks, g.Output, sink)` — goroutine que higieniza o streaming (ver abaixo) e devolve um novo canal de `Chunk`.

**Output scrubber** (`scrub`): goroutine com buffer de borda.
```
K = 48
buffer := ""
para cada chunk recebido:
    se chunk.Err != nil || chunk.Done: emite scrub(buffer) restante; repassa o chunk; encerra
    buffer += chunk.Content
    se len(buffer) > K:
        commit := buffer[:len(buffer)-K]; buffer = buffer[len(buffer)-K:]
        s, finds := PIIscrub(commit)
        para cada find: emite guard_out
        repassa Chunk{Content: s, Provider: chunk.Provider}
ao fechar o canal de entrada sem Done: emite scrub(buffer) restante.
respeita ctx.Done() para encerrar (cliente desconectou).
```
O scrubber reusa a lógica de mascaramento do `PIIGuard` (ex: `guardrail.ScrubPII(text) (string, []Finding)` exportada e usada tanto pelo `PIIGuard.Inspect` quanto pelo scrubber).

---

## Eventos novos (mesmo `event.Sink`)

| Type | Quando | Dashboard |
|---|---|---|
| `guard_in` | finding de PII no input | estágio ① + linha na timeline |
| `masked_prompt` | após mascarar input (`Content`=texto mascarado) | mostra "prompt→LLM" |
| `blocked` | injeção detectada (`Detail`=motivo) | ① vermelho, pipeline para |
| `intent` | classificação (`Detail`=razão) | estágio ② |
| `guard_out` | finding de PII na resposta | estágio ④ + timeline |

Os eventos de corrida (`start`, `provider_start`, `won`, `cancelled`, `chunk`, `done`, `failed`, `error`, `decision`) continuam iguais (estágio ③).

---

## Endpoints e handlers (modificados)

- **`POST /query`** e **`GET /viz/stream`** passam a chamar `gateway.Process(...)` em vez de `router.Dispatch(...)` direto.
  - `/query`: lê `strategy` do body como override (ou `auto`). O `prompt` passado ao `Process` é o conteúdo da **última mensagem com `role=="user"`** (simplificação consciente: guardrail/intenção operam sobre a mensagem do usuário; a conversa multi-turno completa fica para um spec futuro). Se `Process` retorna `ErrBlocked` → **HTTP 403** com `{"error": "...", "reason": "..."}`. Senão `stream.Write` dos chunks higienizados.
  - `/viz/stream`: lê `strategy` da query (`fastest`/`cheapest`/`fallback`/`auto`/vazio). `Process` com o `ChanSink`. Se `ErrBlocked`, o evento `blocked` já foi emitido; o handler fecha o sink e escreve `[DONE]`.
- **`GET /viz/stream`** aceita `strategy=auto` (novo valor) além dos existentes; valor inválido → 400.
- **`POST /v1/chat/completions`**: inalterado (router direto) neste spec.
- **`server.Server`** ganha campo `Gateway *pipeline.Gateway`; `server.New(r, sabotage, gateway)`.
- **`main.go`** monta `gateway := &pipeline.Gateway{Input: guardrail.Chain{guardrail.NewInjectionGuard(), guardrail.NewPIIGuard()}, Output: guardrail.NewPIIGuard(), Router: r}` e injeta.

---

## Dashboard (`server/static/`)

Acima das pistas, uma faixa **PIPELINE** com 4 estágios que acendem conforme os eventos chegam:
```
① Guard In   — verde "N PII mascarados (tipos)" ou vermelho "BLOQUEADO: motivo"
   prompt→LLM: <texto mascarado>
② Intent     — "complex → fastest (razão)"
③ Race       — as pistas que já existem
④ Guard Out  — "N PII na resposta" (ou "limpo")
```
- Novo modo no seletor de estratégia: **`auto`** (default), além de fastest/cheapest/fallback.
- `app.js`: novos `case` no `handle()` para `guard_in`, `masked_prompt`, `blocked`, `intent`, `guard_out`, atualizando a faixa e a timeline (todo texto vindo do servidor passa por `esc()`).
- `blocked` → ① vermelho, demais estágios apagados, nenhuma pista criada.

---

## Tratamento de erros

| Situação | Comportamento |
|---|---|
| Injeção no input | `/query` → 403 `{error,reason}`; `/viz/stream` → evento `blocked` + `[DONE]` |
| PII no input/output | mascarado; nunca derruba a requisição |
| Prompt vazio (`q`/messages) | 400 (como hoje) |
| Estratégia inválida na query | 400 |
| Todos providers falham | evento `error` (como hoje) |
| Cliente desconecta | `ctx` cancela; scrubber e router encerram sem leak |
| Scrubber recebe `Err`/`Done` | descarrega buffer restante higienizado e repassa o chunk terminal |

Erros nunca viram panic; bloqueio é um valor (`ErrBlocked`), não exceção.

---

## Testes (`go test ./...`, stdlib só)

| Pacote | Cobertura |
|---|---|
| `guardrail/` | `PIIGuard` mascara cada tipo (email/cpf/phone/card), numera placeholders, conta múltiplas ocorrências, não bloqueia; `InjectionGuard` bloqueia em cada padrão e passa texto limpo; `Chain` para no primeiro bloqueio e thread-a o texto mascarado; `ScrubPII` idempotência em texto sem PII |
| `intent/` | `Classify` → simple/`cheapest` para transacional, complex/`fastest` para profundidade, tiebreaker por tamanho, `Reason` não vazia, case-insensitive |
| `pipeline/` | `Process` emite `guard_in`/`masked_prompt`/`intent`; injeção → `ErrBlocked` + evento `blocked` e **nenhuma** chamada ao provider (provider mock que registra se foi chamado); o **prompt mascarado** chega ao router (provider mock captura `Request.Messages`); output com PII é higienizado no canal de saída; PII partido entre 2 chunks é capturado pelo buffer de borda |
| `server/` | `/query` → 403 em injeção; `/query` com PII no body → o que chega ao provider está mascarado (mock captura); `/viz/stream` emite `guard_in`+`intent`+`[DONE]`; `strategy=auto` aceito; estratégia inválida → 400; testes de produção existentes seguem verdes |

**Trade-offs documentados (para a banca):**
- Regex < NER contextual (Presidio) — não pega nomes próprios; pega formatos canônicos.
- Buffer de borda no output resolve a maioria dos cortes entre chunks; um token cruzando exatamente o limite num round pode escapar (alternativas — bufferizar tudo ou NER streaming — têm custo pior para o objetivo didático).
- Intenção por heurística < classificação semântica por embeddings; latência ~µs e zero dependência.

---

## Módulo Go

Permanece `github.com/domiciano/llm-proxy`, **sem dependências externas**. Mantém o argumento: Go resolve o gateway inteiro com primitivas nativas (regex da stdlib, goroutines, channels).
