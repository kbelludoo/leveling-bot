# 🤖 AI_INSTRUCTIONS.md — MANUAL DE ENGENHARIA PARA IAs

> **DESTINATÁRIO:** Qualquer modelo ou agente de IA (Antigravity, Claude, ChatGPT, etc.) encarregado de criar ou modificar um bot de jogo em Go para o **dj**.
> **LEIA ESTE ARQUIVO ANTES DE TOCAR EM QUALQUER LINHA DE CÓDIGO.**

---

## 🎯 OBJETIVO PRINCIPAL
Você está construindo um bot autônomo nativo em **Go (Golang)** de altíssima performance e baixo consumo de recursos para um jogo de navegador/idle.

**Invariantes Inegociáveis:**
1. **Cada Jogo é um Binário Independente:** Nunca misture múltiplos jogos no mesmo executável. Se o jogo X cair ou mudar de protocolo, os outros jogos não podem ser afetados.
2. **Consumo de Memória Cirúrgico:** O processo deve consumir **menos de 15 MB de RAM** (geralmente entre 2 e 5 MB). Nunca use browsers headless (Puppeteer/Playwright/Chromium) na VPS para rodar o loop de jogo. Conecte diretamente via WebSockets ou HTTP.
3. **Resiliência Total contra Crash:** Toda conta roda em Goroutine protegida com `engine.RunWithSupervisor` e `recover()`. Um panic de unmarshal ou erro de rede NUNCA deve derrubar o executável principal.
4. **Isolamento de Portas Web:** Cada bot tem sua própria porta no dashboard embutido (ex: Huntera usa 3000, Tibidle usa 3001, Sigma usa 3002). Nunca crie conflito de portas.
5. **Segurança de Segredos:** `accounts.json`, cookies, senhas e chaves SSH (`*.key`) NUNCA podem ser adicionados ao Git. O `.gitignore` deste template já está calibrado.

---

## 📁 ESTRUTURA DO PROJETO E RESPONSABILIDADES

```text
bot-template-go/
├── main.go                     # Entrypoint universal (carrega JSON, inicia bots, web server e console ticker)
├── accounts.json               # Credenciais reais (IGNORADO PELO GIT)
├── accounts.example.json       # Exemplo limpo para o usuário
├── go.mod / go.sum             # Módulo Go nativo (dependência única: gorilla/websocket)
│
├── pkg/
│   ├── engine/                 # [NÃO MODIFICAR A MENOS QUE NECESSÁRIO]
│   │   ├── bot.go              # Interface GameBot e struct AccountStatus
│   │   ├── manager.go          # Gerenciador concorrente (thread-safe) de contas
│   │   └── supervisor.go       # Goroutine runner com backoff e proteção contra panic
│   │
│   ├── network/                # [NÃO MODIFICAR A MENOS QUE NECESSÁRIO]
│   │   ├── socket.go           # Cliente WebSocket resiliente, thread-safe (writeMu), keepalive
│   │   └── http_client.go      # Cliente HTTP com headers de browser moderno e helpers JSON
│   │
│   ├── auth/                   # [USAR SE O JOGO EXIGIR CLOUDFLARE/TURNSTILE]
│   │   └── cdp_ticket.go       # CDP launcher temporário local para resolver desafios/tickets
│   │
│   ├── server/                 # [NÃO MODIFICAR]
│   │   ├── server.go           # Servidor HTTP leve para /api/status, /api/action, /api/ticket
│   │   └── index.html          # Web Dashboard Cyberpunk/Dark embutido (//go:embed)
│   │
│   └── game/                   # ⭐ [AQUI FICA TODA A LÓGICA DO NOVO JOGO] ⭐
│       ├── types.go            # Structs de pacotes, configurações e dados do jogo
│       ├── catalog.go          # Catálogo de monstros, rotas, itens ou mapas
│       └── bot.go              # Implementação concreta da interface engine.GameBot
```

---

## 🛠️ PASSO A PASSO PARA A IA CRIAR UM NOVO BOT

Sempre que o **dj** pedir para criar um bot para um jogo novo, siga esta esteira de engenharia rigorosa:

### Passo 1: Reconhecimento do Jogo (Engenharia Reversa)
1. **Inspecione a aba Network (DevTools) do jogo:**
   - O jogo se comunica via **WebSocket (`ws://` ou `wss://`)** ou **HTTP Polling (`GET/POST`)**?
   - Como é feito o Login? (Cookie de sessão, Bearer Token JWT ou envio de e-mail/senha em endpoint REST?)
   - Há proteção Cloudflare / Turnstile? (Se houver ticket efêmero, use a estratégia CDP do `pkg/auth/cdp_ticket.go`).
2. **Mapeie os Pacotes Principais:**
   - Pacote de handshake/autenticação inicial (ex: `{ type: "auth", token: "..." }`).
   - Pacote de dados do personagem (HP, Mana, Level, Ouro, Inventário).
   - Pacote de ação de caça/combate/movimento (ex: `start_hunt`, `attack`, `travel`).
   - Pacote de finalização de caça ou recompensas (`ended`, `loot`).

### Passo 2: Implementar a Pasta `pkg/game/`
Mantenha `pkg/engine/`, `pkg/network/` e `pkg/server/` intactos. Concentre **100% do código específico** dentro de `pkg/game/`:

1. **`pkg/game/types.go`:**
   - Modele as structs Go com as tags `json:"..."` correspondentes aos pacotes reais interceptados.
   - Ajuste `AccountConfig` e `GameSettings` com as opções do jogo (ex: `HuntTarget`, `AutoHeal`, `Lure`).
2. **`pkg/game/catalog.go` (Se houver mapas/monstros):**
   - Extraia o JSON de catálogo do client do jogo e embuta com `//go:embed catalog.json`.
3. **`pkg/game/bot.go` (Implementar `engine.GameBot`):**
   - **`Init(ctx)`:** Valida cookies/tokens ou realiza o login HTTP inicial.
   - **`Run(ctx)`:** Conecta ao WebSocket via `network.NewSocketClient` ou inicia o loop de ações. Registre os handlers de mensagens recebidas.
   - **`GetStatus()`:** Garanta leitura sob `sync.RWMutex` e preencha HP, Mana, Level, Gold, Kills e EXP/h.
   - **`ExecuteAction(action, payload)`:** Trate ações manuais vindas do Web Dashboard (ex: forçar cura, mudar área, vender inventário).

### Passo 3: Configurar e Testar Localmente
1. Atualize `accounts.example.json` com campos amigáveis e documentados.
2. Compile o código:
   ```bash
   go mod tidy
   go build .
   ```
3. Garanta que o terminal exiba o monitor limpo a cada 4 segundos com memória RAM e goroutines.

### Passo 4: Preparar Deploy para Linux / VPS
1. O executável deve ser cross-compilado para Linux x86_64:
   ```cmd
   cmd /c "set GOOS=linux&& set GOARCH=amd64&& go build -ldflags=\"-s -w\" -o bot-app-linux ."
   ```
2. Configure a porta web única no `main.go` ou via variável de ambiente `PORT` (ex: 3001, 3002).
3. Entregue o arquivo `.service` para o systemd.

---

## ⚠️ ARMADILHAS COMUNS QUE A IA DEVE EVITAR
- ❌ **NÃO crie dependências de CGO:** Mantenha `CGO_ENABLED=0` para que o binário rode em qualquer distro Linux sem exigir bibliotecas glibc externas.
- ❌ **NÃO faça escrita concorrente no WebSocket:** Sempre use o método `s.SendJSON()` ou `s.SendText()` do `pkg/network/socket.go` porque ele já protege contra concorrência com mutex (`writeMu`).
- ❌ **NÃO deixe loops sem select em `ctx.Done()`:** Todo loop `for` com ticker deve escutar `case <-ctx.Done(): return` para permitir encerramento limpo via SIGTERM.
