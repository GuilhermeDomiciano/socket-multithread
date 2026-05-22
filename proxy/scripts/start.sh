#!/bin/bash
# Sobe o proxy. Carrega .env se existir.
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$SCRIPT_DIR"

if [ -f .env ]; then
  set -a
  source .env
  set +a
  echo ">> .env carregado"
else
  echo "AVISO: .env não encontrado. Copie .env.example para .env e preencha as chaves."
  exit 1
fi

echo ">> Iniciando proxy na porta ${PROXY_PORT:-8080} (strategy=${PROXY_STRATEGY:-fastest})"
go run main.go
