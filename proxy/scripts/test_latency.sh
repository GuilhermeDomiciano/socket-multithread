#!/bin/bash
# Compara latência entre as 3 estratégias para o mesmo prompt.
# O proxy precisa estar rodando antes: ./scripts/start.sh

BASE="http://localhost:${PROXY_PORT:-8080}"
PROMPT='{"messages":[{"role":"user","content":"Responda só: oi"}],"max_tokens":5}'

echo "Comparando latência das estratégias (mesmo prompt, 3 execuções cada)..."
echo

for strategy in fastest cheapest fallback; do
  echo -n "  $strategy: "
  payload=$(echo "$PROMPT" | sed "s/}$/,\"strategy\":\"$strategy\"}/")
  total=0
  for i in 1 2 3; do
    ms=$(curl -s -o /dev/null -N -w "%{time_total}" -X POST "$BASE/query" \
      -H "Content-Type: application/json" \
      -d "$payload")
    ms=$(echo "$ms * 1000 / 1" | bc 2>/dev/null || echo "${ms}000" | cut -d. -f1)
    echo -n "${ms}ms "
  done
  echo
done

echo
echo ">> Fastest deve ser ≤ cheapest ≤ fallback em média."
