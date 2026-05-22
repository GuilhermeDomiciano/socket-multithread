# LLM Proxy Paralelo — Design Spec

**Data:** 2026-05-22  
**Projeto:** Sistemas Paralelos — ULBRA  
**Stack:** Go, goroutines, channels, context, net/http

---

## Objetivo

Implementar um proxy HTTP em Go que distribui requisições para múltiplos provedores de LLM (OpenAI, Anthropic, Gemini) usando paralelismo real via goroutines. O projeto serve dois propósitos simultâneos: demonstrar conceitos de sistemas paralelos (CSP, fan-out, cancelamento por contexto) e produzir um sistema funcional utilizável.

---

## Estrutura de Pacotes

```
proxy/
├── main.go                  # entrypoint, inicializa server com providers injetados
├── server/
│   ├── server.go            # HTTP server, registro de rotas
│   ├── handler_query.go     # POST /query  (API própria)
│   └── handler_openai.go    # POST /v1/chat/completions  (compatível OpenAI)
├── router/
│   ├── router.go            # seleciona e executa a estratégia configurada
│   ├── strategy_fastest.go  # fan-out paralelo, retorna o primeiro a responder
│   ├── strategy_cheapest.go # escolhe provedor por custo estimado de tokens
│   └── strategy_fallback.go # tentativa sequencial por ordem de prioridade
├── provider/
│   ├── provider.go          # interface Provider, tipos Request e Chunk
│   ├── openai.go
│   ├── anthropic.go
│   └── gemini.go
└── stream/
    └── writer.go            # consome chan Chunk e escreve SSE no ResponseWriter
```

---

## Interface Provider

```go
type Provider interface {
    Name() string
    CostPer1kTokens() float64
    Stream(ctx context.Context, req Request, out chan<- Chunk) error
}

type Request struct {
    Messages    []Message
    MaxTokens   int
    Temperature float64
}

type Message struct {
    Role    string // "user" | "assistant" | "system"
    Content string
}

type Chunk struct {
    Content  string
    Provider string
    Err      error
    Done     bool
}
```

Cada provider escreve chunks em `out` até concluir ou até `ctx` ser cancelado. Um chunk com `Err != nil` sinaliza falha; `Done: true` sinaliza fim normal do stream.

---

## Fluxo de Dados

```
HTTP Request
     │
  server/          ← valida e normaliza para Request interno
     │
  router/          ← lê PROXY_STRATEGY, cria ctx com timeout
     │
  ┌──┴──────────────────────────┐
  │  goroutines por provider     │   (quantidade depende da estratégia)
  │  provider.Stream(ctx, ...)   │
  └──┬──────────────────────────┘
     │  chan Chunk
  stream/writer    ← consome canal, escreve SSE no ResponseWriter
     │
HTTP Response (streaming, Server-Sent Events)
```

---

## Estratégias de Roteamento

### Fastest-wins
Abre uma goroutine por provider com o mesmo `ctx`. O router cria um canal de chunks único. O primeiro provider a enviar o primeiro chunk válido "vence": o router chama `cancel()`, sinalizando `ctx.Done()` para todos os demais. Os providers param ao detectar o cancelamento. Demonstra o padrão canônico de first-response-wins com `context.WithCancel`.

### Cheapest
Antes de qualquer chamada de rede, calcula o custo estimado de cada provider:  
`custo = (len(prompt) / 4) * provider.CostPer1kTokens() / 1000`  
Seleciona o provider de menor custo e dispara apenas ele. Se falhar, recai em fallback sequencial. Demonstra decisão baseada em metadados sem overhead de rede adicional.

### Fallback
Itera sequencialmente pela lista `PROXY_FALLBACK_ORDER`. Para cada provider, cria um `context.WithTimeout` independente. Se o provider retornar erro ou expirar, avança para o próximo. Demonstra resiliência e contrasta diretamente com o paralelismo do fastest-wins.

---

## Configuração

Todas as configurações via variáveis de ambiente — sem arquivos de config externos:

| Variável | Valores | Padrão |
|---|---|---|
| `PROXY_STRATEGY` | `fastest`, `cheapest`, `fallback` | `fastest` |
| `PROXY_TIMEOUT_MS` | inteiro (ms) | `5000` |
| `PROXY_PORT` | inteiro | `8080` |
| `PROXY_FALLBACK_ORDER` | csv de nomes | `openai,anthropic,gemini` |
| `OPENAI_API_KEY` | string | — |
| `ANTHROPIC_API_KEY` | string | — |
| `GEMINI_API_KEY` | string | — |

---

## Endpoints

### `POST /query` (API própria)

Request:
```json
{
  "messages": [{"role": "user", "content": "Explique goroutines"}],
  "strategy": "fastest",
  "max_tokens": 512
}
```

Response (SSE):
```
data: {"content":"Goroutines são","provider":"openai"}\n\n
data: {"content":" threads leves...","provider":"openai"}\n\n
data: [DONE]\n\n
```

O campo `strategy` no body sobrepõe `PROXY_STRATEGY` para aquela requisição.

### `POST /v1/chat/completions` (compatível OpenAI)

Aceita o mesmo payload do OpenAI Chat Completions. Quando `stream: true`, retorna SSE no formato OpenAI (`delta.content`). Permite usar qualquer SDK OpenAI sem modificação, apontando a base URL para o proxy.

---

## Streaming (stream/writer)

O `writer` consome `chan Chunk` e escreve no `http.ResponseWriter`:

- Define headers `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `X-Accel-Buffering: no`
- Usa `http.Flusher` para flush imediato a cada chunk
- Quando `chunk.Done == true`, escreve `data: [DONE]` e encerra
- Quando `chunk.Err != nil`, escreve `data: {"error":"..."}` e encerra
- Monitora `req.Context().Done()` para encerrar se o cliente desconectar — o mesmo mecanismo cancela os providers upstream

---

## Tratamento de Erros

| Camada | Situação | Comportamento |
|---|---|---|
| Provider | timeout ou erro HTTP | envia `Chunk{Err: err}`, encerra goroutine |
| Router (fastest) | todos os providers falharam | retorna erro 502 ao server |
| Router (cheapest) | provider escolhido falhou | tenta fallback sequencial |
| Router (fallback) | provider atual falhou | avança para o próximo; 502 se esgotar |
| Server | request malformado | retorna 400 antes de entrar no pipeline |
| Server | chave de API ausente | retorna 503 com provider identificado |

Erros fluem como valores pelo canal (`Chunk.Err`), nunca como panics.

---

## Testes

**Unitários** — cada pacote testável isoladamente via injeção de interface:

```go
type MockProvider struct {
    name     string
    delay    time.Duration
    chunks   []string
    failWith error
}
```

| Pacote | Cobertura |
|---|---|
| `provider/` | serialização de request, parse de chunks SSE de cada API |
| `router/fastest` | retorna chunks do provider mais rápido, cancela os demais |
| `router/cheapest` | seleciona o provider de menor custo calculado |
| `router/fallback` | avança para próximo provider ao receber erro |
| `stream/writer` | formato SSE correto, `[DONE]` ao fim, erro tratado |
| `server/` | integração HTTP→SSE completa com mocks via `httptest` |

**Integração com APIs reais** — arquivo `integration_test.go` com build tag `//go:build integration`, executado manualmente com chaves reais. Não roda em `go test ./...` padrão.

Ferramenta: `go test ./...` — zero dependência externa além da stdlib.

---

## Módulo Go

```
module github.com/domiciano/llm-proxy

go 1.22
```

Sem dependências externas — apenas stdlib. Justificado pelo objetivo didático de mostrar que Go resolve o problema com primitivas nativas.
