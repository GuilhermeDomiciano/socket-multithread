# Apresentação "Race Briefing" — rota `/apresentacao`

**Data:** 2026-06-12 · **Status:** aprovado · **Urgência:** apresentação é hoje

## Objetivo

Substituir o PPTX (`Smart-LLM-Gateway.pptx`) por uma página HTML servida pelo próprio gateway,
com 100% do conteúdo dos 16 slides, no DNA visual do dashboard "Parallel GP". Formato
scrollytelling: rola como página, navega como deck. Não é PowerPoint nem tem cara de IA —
parece uma extensão do produto.

## Decisões (com o usuário)

- **Formato:** scrollytelling + teclas (seções fullscreen com scroll-snap; setas navegam).
- **Conteúdo vivo:** sim — 1 widget embutido (guardrails ao vivo) + deep-links pro dashboard.
- **Animações:** CSS puro, transições fluidas, reveal por seção.
- **Rota:** dentro do projeto, `GET /apresentacao`.

## Arquitetura

Novos arquivos, nenhuma mudança no caminho de dados:

```
server/static/apresentacao/
  index.html    — as 16 seções ("voltas"), conteúdo PT-BR do PPTX
  deck.css      — tema GP adaptado (tokens copiados de static/styles.css)
  deck.js       — navegação por tecla, HUD de progresso, IntersectionObserver, widget vivo
server/server.go — +1 rota: GET /apresentacao → serve o index.html do embed FS
```

O `staticHandler()` existente não muda. Tudo stdlib.

## Conteúdo — 16 voltas (mapa 1:1 com o PPTX)

1. Capa — Smart LLM Gateway, primitivas Go, zero deps
2. Motivação — latência, disponibilidade, custo, privacidade
3. Visão geral — 1 prompt, N provedores correm, 1ª vence
4. Backend em camadas — server → pipeline → router → provider → scrub; `provider.Chunk`
5. A peça central — Provider, Chunk, contrato do canal (quem abre fecha, CSP)
6. Pipeline — Guard In → Intent → Router → Guard Out; bloqueio = 403, nunca panic
7. Fastest — fan-out, 1ª vence, `cancel()` nos perdedores no mesmo ms ⚡ *diagrama animado*
8. Roteamento — fastest / parallel-all / sequential / fallback / cheapest / auto
9. Teoria → projeto — fan-out/fan-in, worker pool+WaitGroup, cancelamento por contexto, pipeline
10. Benchmark — seq (soma) vs par (máx), ≈2,7× exemplo, Amdahl *diagrama animado*
11. Segurança — chain de guards na entrada, scrub em streaming na saída, buffer de borda 🔴 *PIT STOP vivo*
12. Observabilidade — sink nil em produção, ChanSink no dashboard, regra de ouro
13. Concorrência correta — 4 invariantes + contraexemplo `./run.sh race`
14. Casos de uso I — latência, disponibilidade, custo, LGPD
15. Casos de uso II — anti-injection, OpenAI-compat, A/B de modelos
16. Demo ao vivo — roteiro dos 5 passos + mensagem-chave 🏁 *deep-link dashboard*

## Navegação e HUD

- `←/→`, `Espaço`, `PgUp/PgDn` saltam de seção; scroll livre também funciona.
- `g` abre overlay-índice (grid das 16 voltas) para salto direto.
- HUD fixo: barra de progresso estilo pista + `LAP NN/16` + bandeira quadriculada no fim.
- Scroll-snap (`scroll-snap-type: y mandatory`) para seções sempre enquadradas no projetor.

## Visual e animações

- Tokens do tema GP reaproveitados: asfalto `#0c0d10`, amarelo `#f4c20d`, JetBrains Mono +
  Archivo Narrow, kerb stripes, grain overlay. Projector-first: tipografia grande, alto contraste.
- Reveal por IntersectionObserver: seção ativa anima entrada (stagger nos cards).
- Diagramas animados em CSS:
  - **Fastest (volta 7):** 3 barras de corrida; vencedor pisca `WON`, perdedores esmaecem `cancelado`.
  - **Pipeline (volta 6):** chunk fluindo pelos 4 estágios.
  - **Benchmark (volta 10):** barras seq vs par com proporção ~2,7×.
- `prefers-reduced-motion: reduce` desliga animações.

## Pontos vivos — "PIT STOP"

- **Widget embutido (volta 11, Segurança):** input + botão que faz `POST /query` real.
  - CPF no prompt → resposta mostra `[REDACTED_*]` (PII mascarada).
  - "ignore previous instructions" → mostra o HTTP 403 (InjectionGuard).
  - Uma `fetch` simples; lê o corpo (SSE ou erro) e mostra texto. Falha de rede → mensagem
    discreta no próprio widget, nunca quebra a página.
- **Deep-links (voltas 7, 10, 16):** botão "🏁 Abrir o grid" → abre `/` (dashboard) em nova aba.
  Corrida, sabotagem e benchmark já existem lá — não duplicar.

## Tratamento de erro

- Widget vivo: timeout/erro de fetch exibido inline no widget; resto da página é estático e
  não depende do backend além de ser servido por ele.
- Sem JS (ou erro de JS): conteúdo continua legível por scroll normal — JS só melhora.

## Verificação

- `./run.sh check` limpo (gofmt + vet + build).
- Abrir `http://localhost:<porta>/apresentacao` e percorrer as 16 voltas por tecla e por scroll.
- Testar o widget PIT STOP com CPF e com tentativa de injeção.
- Conferir no projetor (ou zoom do navegador) a legibilidade.

## Fora de escopo

- Modo presenter com notas, export PDF, telemetria da apresentação.
- Duplicar a corrida ao vivo do dashboard dentro da apresentação.
