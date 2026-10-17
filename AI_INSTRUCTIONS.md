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
5. **Segurança Absoluta de Segredos (Zero Hardcoding):** NUNCA insira tokens de API (Telegram, Discord, Twitch OAuth), senhas, cookies, chaves SSH (`*.key`) ou IPs públicos da VPS diretamente em arquivos `.go`, `.json`, `.sh`, `.bat` ou scripts que serão commitados. TODAS as credenciais devem ser obrigatoriamente carregadas via variáveis de ambiente (`os.Getenv`) ou arquivos locais no `.gitignore` (`.secrets.json`, `.env`, `accounts.json`). Commitar tokens gera vazamento público imediato!
6. **Modo Stealth & Anti-Ban (Zero Nicks Públicos):** NUNCA exponha o nome real do personagem/treinador/streamer nos dashboards web do GitHub Pages (`docs/index.html` ou `status.json`). Sempre exiba apenas a classe/vocação, índice anônimo (ex: `Conta #1`, `Treinador #1`, `Guerreiro`, `Arqueiro`) e estatísticas técnicas (Level, EXP/h, Kills, Gold, Mapa). Transmitir nicks reais gera denúncias e banimentos manuais imediatos!
7. **Proteção Anti-Ban Circuit Breaker Obrigatória:** Todo bot deve integrar `pkg/engine.CircuitBreaker`. Diante de qualquer recusa de save, rate-limit ou erro de anti-cheat (`experiência rápida demais`, `recusado: true`), o bot NUNCA deve insistir em novos saves. Deve desarmar o combate, recuar para zona segura e aguardar cooldown de 15 a 30 minutos. Insistir em saves rejeitados resulta em banimento de 7 dias por programa de terceiros!

---

## 📁 ESTRUTURA DO PROJETO E RESPONSABILIDADES

```text
bot-template-go/
├── main.go                     # Entrypoint universal (carrega JSON, inicia bots, web server e console ticker)
├── accounts.json               # Credenciais reais (IGNORADO PELO GIT)
├── accounts.example.json       # Exemplo limpo para o usuário
├── AGENTS.md                   # Diretrizes universais de IA e anti-ban
├── go.mod / go.sum             # Módulo Go nativo (dependência única: gorilla/websocket)
│
├── pkg/
│   ├── engine/                 # [NÃO MODIFICAR A MENOS QUE NECESSÁRIO]
│   │   ├── bot.go              # Interface GameBot e struct AccountStatus
│   │   ├── circuit_breaker.go  # Disjuntor anti-ban, detecção de anti-cheat e cálculo de taxa coletiva
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

## 🧠 REGRAS DE OURO DA IA: GAMEPLAY & ARQUITETURA AUTÔNOMA

Sempre que construir um novo bot usando este template, a IA deve projetar e codificar os **10 Pilares da Eficiência Máxima** aprendidos com o ecossistema real de bots do projeto:

### 1. Algoritmo de Melhor Rota de EXP/h (Dynamic Route Solver)
- **Mineração do Catálogo do Jogo:** A IA deve analisar o código JavaScript/bundle do jogo e extrair todos os monstros, mapas, fraquezas elementais, dano base e requisitos de nível para um catálogo estático ou dinâmico (`pkg/game/catalog.go`).
- **Cálculo da Rota Ótima:**
  - **Fator Nível:** $\text{Nível da Área} \le \text{Nível do Personagem}$ (evitar penalidades ou bloqueios).
  - **Multiplicador Elemental / Fraqueza:** Se o jogo tiver sistema de fraquezas elementais ou tipos (ex: 4.0x no Poke Idle World), o algoritmo deve priorizar alvos onde o dano do bot seja maximizado para matar monstros em 1 ou 2 turnos (aumentando *kills por minuto*).
  - **Risco Defensivo:** Evitar monstros que causem dano super efetivo ou possam nocautear o jogador, eliminando o tempo morto de morte/revive.
  - **Reavaliação Contínua:** Ao subir de nível (Level Up) ou trocar de equipamento, o bot deve recalcular instantaneamente e transitar para a nova melhor rota.

### 2. Auto-Sell Inteligente (Gestão de Inventário)
- Monitorar a capacidade da mochila/inventário.
- Manter uma lista de itens protegidos (itens equipados, itens de tier alto, consumíveis raros).
- Vender automaticamente o lixo/loot acumulado assim que o inventário atingir o limite ou a cada intervalo programado, convertendo em ouro sem travar a caça.

### 3. Auto-Equip e Auto-Upgrade
- Sempre que dropar um equipamento superior ao atual (maior ataque, maior defesa, melhor tier), o bot deve equipar automaticamente.
- Se o jogo tiver sistema de pontos de atributos (força, agilidade, talentos), distribuir automaticamente conforme a vocação/classe configurada.

### 4. Extração Contínua de Informações (Engenharia Reversa Ativa)
- **A IA deve "ler o jogo":** Inspecionar pacotes de rede, analisar o código-fonte client-side (JS/Wasm) e salvar o maior número de tabelas de dados possíveis (`hunts.json`, `items.json`, `monsters.json`).
- Toda informação descoberta deve ser persistida para enriquecer o algoritmo e tornar o bot 100% autônomo sem necessidade de intervenção humana.

### 5. Isolamento Multi-IP para Múltiplas Contas (Anti-Ban)
- Quando o usuário configurar mais de uma conta no mesmo jogo, a IA deve verificar se o jogo bloqueia múltiplos logins no mesmo endereço IP.
- **Implementação:** Suportar o campo `"bindIP": "10.0.0.X"` no `accounts.json`.
- O cliente HTTP e o `websocket.Dialer` devem fazer bind no IP local configurado usando `net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(bindIP)}}` (como usado com sucesso no Pokemon Sigma e Baiak Idle).

### 6. Suporte a Protocolos Binários (Protobuf / Colyseus / MessagePack)
- Nem todo jogo se comunica com JSON em texto simples. Se a inspeção da aba Network revelar pacotes binários:
  - **Protobuf 3:** Usar `google.golang.org/protobuf/encoding/protowire` para ler tags e decodificar campos binários de alta velocidade sem precisar compilar arquivos `.proto` pesados (como no Leveling Idle 2D).
  - **Colyseus / State Sync:** Tratar frames binários (`websocket.BinaryMessage`) lendo schema offsets ou action bytes iniciais.

### 7. Bônus Externos & Lurker Integrado (Twitch & Kick Drops)
- Se o jogo oferecer bônus de EXP/Ouro (+15% a +25%) por assistir streamers ou canais parceiros (como no PokeIdle.io):
  - A IA deve implementar um módulo concorrente em Goroutine que autentica na API da plataforma de streaming (Twitch/Kick) e mantém a presença ativa em background, garantindo o bônus máximo 24/7 sem abrir navegadores pesados.

### 8. Gestão Inteligente de Stamina / Vigor & Modos de Descanso
- Em jogos onde o farm depende de Stamina (ex: Huntera, Tibidle, Baiak):
  - O bot deve calcular o tempo restante de stamina e o custo do refil.
  - Se o farm de ouro cobrir o custo de compra de refil (`staminaRefillCost`), efetuar a recarga automaticamente.
  - Se a stamina esgotar e não houver refis, o bot deve mover o personagem para modo *Treino Online* ou *Descanso* para regeneração acelerada, evitando desperdício de tempo e EXP zerada.

### 9. Cooperação em Grupo (Party & Sinergia de Classes)
- Se o jogo tiver mecânicas de grupo/party (como o Leveling Idle 2D):
  - A IA deve implementar lógica de líder e membros: a Conta 1 cria a sala/party e as outras contas entram na mesma sessão.
  - Sincronizar papéis (ex: Tanker puxa os monstros, Dps causa dano à distância e Healer cura o time), garantindo o bônus de EXP compartilhada.

### 10. Contrato de Telemetria com o VPS Command Center
Todo bot criado deve expor um endpoint HTTP leve em sua porta designada (`GET /api/status`) retornando JSON padronizado para integração nativa com o **Bot Hub**, o **Watchdog** e os **Alertas do Telegram**:
```json
{
  "online": true,
  "uptime": "14h20m",
  "ramAllocMB": 3.2,
  "goroutines": 12,
  "accounts": [
    {
      "id": "acc1",
      "name": "NickDoChar",
      "level": 150,
      "gold": 87000000,
      "kills": 4200,
      "currentArea": "Glooth Bandit",
      "expPerHour": 1450000
    }
  ]
}
```

### 12. Padrão de Validação Estrita de Schema, Anti-Tampering e WebSocket Híbrido (Lições do LoL Idle)

Jogos com sistemas de save avançados em nuvem e multiplayer híbrido (como o LoL Idle / Runeterra) possuem defesas sofisticadas contra bots que quebram implementações ingênuas. Sempre que enfrentar um jogo com essas características, siga estes padrões obrigatórios:

#### A. O Padrão Dynamic Overlay (`RawEstado map[string]any`) — Prevenção de "Save Malformado"
* **O Problema:** A maioria dos bots em Go define uma struct tipada com 10 a 15 campos (`Level`, `Gold`, `Hp`, etc.). No entanto, servidores modernos validam **dezenas de chaves obrigatórias** no schema do JSON (ex: `conquistas`, `skins`, `skin`, `gemas`, `rotina`, `auto`, `autoHab`, `espolios`, `montaria`, `itens`, `mochila`, etc.). Quando a struct Go serializa o JSON no `PUT /api/save`, ela omite os campos desconhecidos. O servidor detecta o schema incompleto e rejeita o progresso com erro silencioso de `"save malformado"`.
* **A Solução:** Nunca envie um JSON derivado apenas de uma struct estática.
  1. No `GET /api/eu`, armazene o JSON bruto do estado em um `RawEstado map[string]any`.
  2. Mantenha os campos de combate tipados para lógica de jogo interna (`Level`, `Exp`, `Gold`).
  3. No momento de salvar (`salvarSave`), faça o **overlay** apenas das propriedades alteradas sobre o `RawEstado` original e envie `{ "estado": raw }`. Isso garante preservação de 100% dos campos que o servidor espera, independentemente de atualizações futuras.

#### B. Isolamento de Instâncias e Headers de Origem (`X-Instancia` e `outraAba`)
* **O Problema:** Para impedir que duas abas ou dois aparelhos salvem ao mesmo tempo gerando conflitos de concorrência, o servidor gera ou exige um identificador único de instância da página (`X-Instancia`).
* **A Regra:** Gere uma string aleatória única na inicialização da conta (`X-Instancia: Math.random() + Date.now()`) e envie esse mesmo header em **todas** as requisições (`GET /api/eu`, `PUT /api/save`, etc.). Mudar de instância entre um GET e um PUT fará o servidor responder com `outraAba: true`, desativando o salvamento.
* **Headers de Fingerprint:** Requisições de save via HTTP REST em jogos web frequentemente exigem cabeçalhos de navegador autênticos: `Origin: https://dominio.com` e `Referer: https://dominio.com/jogar`. Omiti-los pode disparar recusas silenciosas ou bloqueios WAF.

#### C. Controle Otimista de Concorrência (`rev` Tracking)
* O servidor mantém uma chave de revisão atômica no banco de dados (`rev`).
* Toda resposta de `GET /api/eu` ou de `PUT /api/save` bem-sucedido retorna a nova `rev`.
* **Regra:** O bot deve atualizar o campo `rev` imediatamente na memória local. Enviar um save com `rev` antiga resulta em `409 Conflict: Progresso desatualizado`.

#### D. Invariantes Matemáticas e Orçamento de Atributos (Anti-Tampering)
Servidores com validação heurística checam a coerência lógica e matemática do save:
1. **Orçamento Rígido de Atributos:** Os pontos de atributo distribuídos somados aos pontos livres devem respeitar rigorosamente a fórmula matemática:  
   $$\sum \text{atrib} + \text{pontosAtrib} \le \text{PONTOS\_POR\_NIVEL} \times (\text{nível} - 1)$$  
   Se você injetar pontos ou se a simulação acumular além desse teto, o servidor rejeita com `"pontos de atributo demais"`.
2. **Requisitos de Nível em Habilidades:** As habilidades devem respeitar os caps da classe (ex: Ultimate *R* desbloqueada apenas nos níveis 6, 11, 16... e habilidades comuns limitadas a `(nível + 1) / 2`).
3. **Monotonicidade Estrita (Contadores que Não Voltam para Trás):**
   * $\Delta(\text{totais.abates}) \ge 0$
   * $\Delta(\text{totais.ouro}) \ge 0$
   * $\Delta(\text{tempoJogo}) \ge 0$
   * $\Delta(\text{relogio}) \ge 0$  
   Se qualquer um desses valores for inferior ao registrado no banco, o servidor aborta com `"contadores voltaram para trás"` ou `"relógio do jogo inválido"`.
4. **Coerência Econômica:** O ouro possuído nunca pode exceder o total histórico ganho somado ao nível (`totais.ouro - gasto <= ouro`), impedindo injeções mágicas de moedas (`"ouro demais"`).

#### E. Arquitetura Híbrida: REST para Saves + WebSocket para Presença e Party
Em jogos onde o mundo aberto possui modo cooperativo (party/grupo):
* **Separação de Camadas:** A persistência definitiva ocorre via REST (`PUT /api/save`), enquanto a presença em tempo real, visualização de aliados e buffs de grupo ocorrem no WebSocket.
* **Pré-requisito de Localização:** Para que convites de grupo (`{t: "grupo", acao: "convidar"}`) sejam aceitos, ambos os personagens devem estar fisicamente no mapa multiplayer correspondente (`local: "arena"` / Runeterra) e transmitindo frames de posição (`{t: "pos"}`) a ~10 Hz. Convites enviados para personagens no modo offline/Rift retornam `"{nome} não pode entrar no grupo agora"`.
* **Distribuição de EXP em Grupo:** Quando em grupo, cada monstro abatido deve transmitir o frame de abate (`{t: "abateGrupo", id: monstro.id}`) via WebSocket para que o servidor oficial calcule e credite a parcela de EXP compartilhada aos membros próximos (+45% de bônus).

#### F. Regra de Concorrência de Conta Única
* O servidor derruba sessões com `401 Unauthorized` se houver múltiplas conexões simultâneas na mesma conta.
* Para formar uma party de 4 jogadores, utilize **4 contas distintas cadastradas**, preferencialmente isoladas por IPs privados/secundários da VPS (`bindIP: 10.0.0.X`) para máxima discrição.

#### G. Padrão Circuit Breaker de Anti-Cheat (Lição Crítica de Prevenção de Ban)
* **O Gatilho da Suspensão:** Servidores com anti-cheat server-side rastreiam a taxa de erros heurísticos (ex: `"experiência rápida demais para este personagem"`). Se um bot continuar caçando e insistir em múltiplos `PUT /api/save` enquanto o servidor estiver recusando o progresso, o sistema automatizado de segurança detecta a discrepância acumulada e aplica suspensão temporária de 7 dias com o motivo `"uso de programas de terceiros"`.
* **A Regra do Disjuntor (Circuit Breaker):**
  1. Ao receber qualquer resposta com `recusado: true` ou erro de taxa/limite, o bot deve **desarmar o combate imediatamente** (`AutoHunt = false`).
  2. Forçar recuo para zona segura/Fonte (`local: "fonte"`).
  3. Resfriamento obrigatório: **pausar novos saves por 15 a 30 minutos** até o relógio real do servidor avançar e absorver a margem temporal.
  4. Recarregar o estado canônico exclusivamente via `GET /api/eu`.
* **Cálculo da Taxa Coletiva em Party (Collective Throttling):**
  Em grupos onde cada abate compartilha EXP (+45% de bônus via frames de WebSocket), a soma das mortes de todos os integrantes canaliza XP em massa para cada membro. A taxa individual deve ser rigorosamente balanceada para que a soma nunca ultrapasse a produtividade de um jogador solo:
  $$\text{KillRate}_{\text{individual}} \le \frac{\text{KillRate}_{\text{solo\_seguro}}}{N_{\text{membros}} \times (1 + \text{BônusGrupo})}$$
  Exemplo: se o teto humano solo for 40 monstros/minuto, em um grupo de 4 com bônus de 45%, cada bot deve matar no máximo $40 / (4 \times 1.45) \approx 7\text{ monstros/minuto}$. Taxas superiores acumulam XP mais rápido do que o relógio físico permite salvar.

---

## ⚠️ ARMADILHAS COMUNS QUE A IA DEVE EVITAR
- ❌ **NÃO crie dependências de CGO:** Mantenha `CGO_ENABLED=0` para que o binário rode em qualquer distro Linux sem exigir bibliotecas glibc externas.
- ❌ **NÃO faça escrita concorrente no WebSocket:** Sempre use o método `s.SendJSON()` ou `s.SendText()` do `pkg/network/socket.go` porque ele já protege contra concorrência com mutex (`writeMu`).
- ❌ **NÃO deixe loops sem select em `ctx.Done()`:** Todo loop `for` com ticker deve escutar `case <-ctx.Done(): return` para permitir encerramento limpo via SIGTERM.


