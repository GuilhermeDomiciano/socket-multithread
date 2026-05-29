---
stepsCompleted: [1, 2, 3, 4]
inputDocuments: []
session_topic: 'Aprimorar o LLM Proxy Paralelo (Go) para apresentação final da disciplina de Sistemas Paralelos'
session_goals: 'Gerar ideias para visualização/UI além do terminal, completar funcionalidades faltantes e maximizar impacto na apresentação'
selected_approach: 'ai-recommended'
techniques_used: ['What If Scenarios']
ideas_generated: ['Corrida dos Provedores', 'Botão de Sabotagem', 'Clímax da Sabotagem', 'Timeline/Replay', 'Raio-X de Goroutines', 'Race Condition Ao Vivo', 'Modo Comparação de Estratégias', 'Viz de Custo (cheapest)', 'Health-check ao vivo', 'Economia acumulada', 'Equalizador de chunks', 'Placar de mortes']
context_file: ''
session_active: false
workflow_completed: true
---

# Brainstorming Session Results

**Facilitador:** Domiciano
**Data:** 2026-05-29

## Session Overview

**Topic:** Aprimorar o LLM Proxy Paralelo (Go) para apresentação final da disciplina de Sistemas Paralelos
**Goals:** Gerar ideias para visualização/UI (hoje só terminal), completar o que falta, e tornar a apresentação impactante mostrando paralelismo real (goroutines, channels, context cancellation, fan-out)

### Session Setup

_Projeto: proxy HTTP em Go que distribui requisições para múltiplos LLMs (OpenAI, Anthropic, Gemini) com estratégias de roteamento (fastest-wins, cheapest, fallback). Demonstra CSP, fan-out e cancelamento por contexto. Stack: Go stdlib apenas, SSE streaming._

## Technique Selection

**Approach:** AI-Recommended Techniques
**Contexto da análise:** apresentação final + necessidade de visualização + provar paralelismo real
**Sequência planejada:** What If Scenarios → Role Playing → SCAMPER
**Executado:** What If Scenarios (sessão encerrada cedo por boa cobertura de ideias — usuário satisfeito com o material gerado e pediu recomendação direta)

## Technique Execution Results — What If Scenarios

Geramos ideias por provocações "E se...?", com o usuário reagindo e filtrando. Ideias capturadas no formato [Categoria #X]:

**[Viz #1]: A Corrida dos Provedores** ✅ ESCOLHIDA
_Conceito:_ Dashboard ao vivo com 3 barras (OpenAI/Anthropic/Gemini) preenchendo em paralelo; a vencedora cruza a linha e as outras desbotam com label "cancelled via context ❌".
_Novidade:_ transforma `context.WithCancel` (abstrato) em evento visual assistível em tempo real.

**[Demo #2]: O Botão de Sabotagem** ✅ ESCOLHIDA
_Conceito:_ painel com botões para injetar falha/latência ao vivo ("derruba o OpenAI", "+5s no Gemini"); a plateia vê o fastest-wins re-rotear sozinho e o fallback avançar na fila.
_Novidade:_ prova resiliência ao vivo em vez de afirmar em slide — teatro de engenharia do caos.

**[Demo #3]: Clímax da Sabotagem** ✅ ESCOLHIDA
_Conceito:_ derrubar a vencedora no meio do stream; o sistema reelege outra na hora sem o usuário final perceber.
_Novidade:_ momento "uau" da apresentação.

**[Viz #4]: Timeline / Replay** ✅ ESCOLHIDA
_Conceito:_ linha do tempo em ms mostrando disparo → 1º chunk → cancel → ctx.Done detectado, pausável para explicar.
_Novidade:_ permite narrar os microssegundos do cancelamento.

**[Viz #5]: Raio-X de Goroutines** ⏸️ bônus
_Conceito:_ contador de goroutines vivas subindo a 3 e despencando a 1 no cancel.

**[Demo #6]: Race Condition Ao Vivo** ✅ ESCOLHIDA (segmento à parte)
_Conceito:_ rodar versão buggada (`go run -race` → DATA RACE / concurrent map writes) e depois a correta.
_Novidade:_ prova domínio teórico do porquê do código concorrente correto.

**Outras ideias geradas (bônus / descartadas):**
- Equalizador de chunks SSE (pulso por provider) — bônus
- Placar de mortes (goroutines canceladas acumuladas) — bônus
- Seletor de estratégia ao vivo (fastest/cheapest/fallback) — bônus
- Modo comparação lado a lado das 3 estratégias — bônus
- Viz do "raciocínio" do cheapest (custos calculados) — bônus
- Economia acumulada em $ — bônus
- Health-check ao vivo dos providers (bolinhas verde/vermelho) — bônus
- ❌ TUI estilo htop (descartado — usuário preferiu web)
- ❌ Legendas com termos acadêmicos na tela (descartado)
- ❌ QR code / prompt pelo celular da plateia (descartado)

**Energia/engajamento:** alta no início, reações rápidas e decisivas filtrando o que servia para a apresentação; encerrou pedindo recomendação direta.

## Idea Organization and Prioritization

### Organização temática

**Tema 1 — Visualizar o paralelismo (o coração)**
A Corrida dos Provedores, Raio-X de Goroutines, Equalizador de chunks, Placar de mortes.
_Insight:_ todos tornam visível o que hoje é invisível: goroutines concorrentes e seu ciclo de vida.

**Tema 2 — Provar resiliência e cancelamento (o clímax)**
Botão de Sabotagem, Clímax da Sabotagem, Timeline/Replay, Race Condition ao vivo.
_Insight:_ demonstram `context.WithCancel`, re-roteamento e a importância da concorrência correta — ao vivo, não em slide.

**Tema 3 — Inteligência de roteamento (bônus de produto)**
Seletor de estratégia ao vivo, Modo comparação, Viz de custo do cheapest, Economia acumulada, Health-check.
_Insight:_ dão ar de "produto real" e conectam paralelismo a valor, mas não movem a nota.

### Decisões de arquitetura tomadas na sessão
- **Web** (HTML + JS puro, sem framework) — coerente com "Go stdlib, zero dependência" e baixo risco de quebrar ao vivo.
- A página **consome o SSE que o proxy já produz** — reaproveita o backend existente.
- Sem TUI, sem legendas acadêmicas na tela, sem prompt via celular.

### Priorização final (esforço × impacto)

| Prioridade | Item | Esforço |
|---|---|---|
| 🥇 Núcleo | A Corrida (3 barras + cancelamento visual) | Médio |
| 🥈 Clímax | Botões de sabotagem + re-roteamento ao vivo (endpoint de injeção de falha) | Médio |
| 🥉 Fechamento | Timeline/Replay pós-corrida em ms | Baixo-médio |
| ➕ Quase grátis | Race condition proposital (`go run -race`, branch buggada) | Baixo |
| ⏸️ Bônus | Raio-X de goroutines, viz de custo, comparação, health-check | — |

## Action Planning

### MVP de apresentação — UMA página web alimentada pelo SSE existente

**Próximos passos:**
1. Criar endpoint/handler que exponha metadados por chunk (provider, timestamp, evento de cancelamento) para o front consumir além do conteúdo.
2. Página `static/index.html` + JS puro consumindo SSE: render das 3 barras animadas (a corrida).
3. Implementar injeção de falha/latência controlável por requisição (flag no body ou endpoint de controle) para os botões de sabotagem.
4. Capturar timestamps no router (disparo, 1º chunk, cancel, ctx.Done por provider) e renderizar a timeline pós-corrida.
5. Manter uma branch/arquivo com data race proposital + script `go run -race` para o segmento didático.

**Roteiro de demo (~5 min):**
1. Mandar pergunta → plateia vê as 3 correndo → uma vence, as outras morrem (paralelismo + cancelamento).
2. Sabotar a vencedora ao vivo → sistema reelege outra (resiliência).
3. Rodar versão com data race → mostrar erro → mostrar versão correta (domínio teórico).
4. Timeline na tela para fechar explicando os microssegundos.

**Métrica de sucesso:** plateia/professor "vê" o paralelismo e o cancelamento acontecendo; nota máxima.

## Session Summary and Insights

**Principais conquistas:**
- Definido o conceito central da visualização: "A Corrida dos Provedores" com cancelamento visual.
- Definido o clímax da apresentação: sabotagem ao vivo + re-roteamento.
- Decisão de stack (web, JS puro) alinhada à filosofia do projeto.
- Roteiro de demo de 5 minutos pronto, do "uau" ao fechamento teórico.

**Reflexões:**
- O usuário tem instinto afiado de produto/apresentação: cortou rápido o que não servia (TUI, QR, legendas) e abraçou o que conta história.
- O backend já existente (SSE) é o ativo a reaproveitar — o trabalho restante é majoritariamente front + um endpoint de injeção de falha + instrumentação de timestamps.
