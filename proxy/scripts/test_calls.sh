#!/bin/bash
# Roda todas as chamadas de teste e mostra as respostas.
# O proxy precisa estar rodando antes: ./scripts/start.sh
set -e

BASE="http://localhost:${PROXY_PORT:-8080}"

sep() { echo; echo "────────────────────────────────────────────────"; echo "  $1"; echo "────────────────────────────────────────────────"; }

# ── 1. /query com fastest (padrão) ───────────────────────────────
sep "1. /query  strategy=fastest  (SSE streaming)"
curl -s -N -X POST "$BASE/query" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Diga olá em uma palavra"}],
    "max_tokens": 20
  }'
echo

# ── 2. /query sobrepondo para cheapest ───────────────────────────
sep "2. /query  strategy=cheapest  (override por requisição)"
curl -s -N -X POST "$BASE/query" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Quanto é 2+2?"}],
    "strategy": "cheapest",
    "max_tokens": 20
  }'
echo

# ── 3. /query sobrepondo para fallback ───────────────────────────
sep "3. /query  strategy=fallback  (override por requisição)"
curl -s -N -X POST "$BASE/query" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Responda só: ok"}],
    "strategy": "fallback",
    "max_tokens": 10
  }'
echo

# ── 4. /v1/chat/completions  streaming=true (formato OpenAI) ─────
sep "4. /v1/chat/completions  stream=true  (formato OpenAI)"
curl -s -N -X POST "$BASE/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Explique goroutines em 1 frase"}],
    "stream": true,
    "max_tokens": 60
  }'
echo

# ── 5. /v1/chat/completions  streaming=false (JSON completo) ─────
sep "5. /v1/chat/completions  stream=false  (JSON completo)"
curl -s -X POST "$BASE/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [{"role": "user", "content": "Diga oi"}],
    "stream": false,
    "max_tokens": 10
  }' | (command -v jq &>/dev/null && jq . || cat)

# ── 6. Validação: messages vazio → 400 ───────────────────────────
sep "6. Validação: messages vazio  (esperado: 400)"
curl -s -w "\nHTTP %{http_code}" -X POST "$BASE/query" \
  -H "Content-Type: application/json" \
  -d '{"messages": []}'
echo

# ── 7. Validação: JSON inválido → 400 ────────────────────────────
sep "7. Validação: JSON inválido  (esperado: 400)"
curl -s -w "\nHTTP %{http_code}" -X POST "$BASE/query" \
  -H "Content-Type: application/json" \
  -d 'isso nao e json'
echo

echo
echo ">> Testes concluídos."
