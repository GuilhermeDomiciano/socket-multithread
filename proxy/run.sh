#!/bin/bash
# run.sh — roda tudo: verifica o código, testa e sobe o proxy com o dashboard web.
#
# Uso:
#   ./run.sh            # verifica + testa + sobe o servidor
#   ./run.sh test       # só roda os testes (inclui -race)
#   ./run.sh check      # só fmt + vet + build (sem subir o servidor)
#   ./run.sh race       # demo didática do data race (go run -race)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

run_check() {
  echo ">> gofmt"
  fmt_out="$(gofmt -l .)"
  if [ -n "$fmt_out" ]; then
    echo "ERRO: arquivos mal formatados:"; echo "$fmt_out"; exit 1
  fi
  echo ">> go vet"
  go vet ./...
  echo ">> go build"
  go build ./...
  echo ">> OK (fmt/vet/build limpos)"
}

run_test() {
  echo ">> go test -race ./..."
  go test -race ./...
}

run_race_demo() {
  echo ">> demo de data race (esperado: WARNING: DATA RACE)"
  go run -race examples/racecondition/main.go || true
}

case "${1:-serve}" in
  check)
    run_check
    ;;
  test)
    run_test
    ;;
  race)
    run_race_demo
    ;;
  serve)
    run_check
    run_test

    if [ -f .env ]; then
      set -a
      source .env
      set +a
      echo ">> .env carregado"
    else
      echo "AVISO: .env não encontrado. Copie .env.example para .env e preencha as chaves."
      exit 1
    fi

    PORT="${PROXY_PORT:-8080}"
    echo ">> Subindo proxy na porta ${PORT} (strategy=${PROXY_STRATEGY:-fastest})"
    echo ">> Dashboard ao vivo:  http://localhost:${PORT}/"
    echo ">> API própria:        POST http://localhost:${PORT}/query"
    echo ">> Compatível OpenAI:  POST http://localhost:${PORT}/v1/chat/completions"
    go run .
    ;;
  *)
    echo "uso: ./run.sh [serve|check|test|race]"
    exit 1
    ;;
esac
