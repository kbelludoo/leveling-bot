# 🛡️ REGRAS GLOBAIS DE ENGENHARIA E ANTI-BAN PARA BOTS EM GO

> **ESCOPO:** Esta regra se aplica a TODOS os bots de jogos desenvolvidos a partir deste template em Go (Golang) (PokeIdle, PokeIdleWorld, LoL Idle, Baiak, Tibia Idle, Sigma, Leveling Idle, etc.).
> **IMPORTÂNCIA:** CRÍTICA / INVIOLÁVEL. Prevenção de suspensões e integridade de contas.

---

## 1. 🚨 PADRÃO OBRIGATÓRIO: CIRCUIT BREAKER DE ANTI-CHEAT (`pkg/engine`)

Todo bot em Go DEVE utilizar o módulo padronizado `pkg/engine.CircuitBreaker`.

### A. Gatilhos de Desarme Imediato (Trip)
O bot deve registrar falha ou desarmar imediatamente (`cb.Trip(...)`) ao detectar qualquer um dos seguintes padrões:
- **HTTP 429** (Too Many Requests / Rate limit)
- **HTTP 403** (Cloudflare / WAF Challenge)
- **Rejeição Heurística do Servidor:**
  - `"experiência rápida demais para este personagem"` (ou qualquer variação)
  - `"pontos de atributo demais"`
  - `"contadores voltaram para trás"` / relógio inválido
  - `"save malformado"`
  - `"recusado": true`
  - `"outraAba": true` (conflito de concorrência)
  - `"banido": true` / aviso de suspensão
- Use a função utilitária nativa: `engine.IsAntiCheatRejection(statusCode, body)`.

### B. Ações Obrigatórias ao Desarmar (Trip):
1. **Desarmar Combate:** Parar de bater ou matar monstros imediatamente (`AutoHunt = false`).
2. **Recuo para Zona Segura:** Mover o personagem para a Fonte, Templo, Cidade ou Depot (`local: "fonte"`).
3. **Pausa Absoluta de Saves (Cooldown de 15 a 30 minutos):** NUNCA insistir em novos `PUT /api/save` ou pacotes de progresso após uma recusa. A insistência é o que dispara o algoritmo de ban automático de 7 dias por "uso de programas de terceiros".
4. **Recuperação em Half-Open:** Quando o cooldown expirar, recarregar o estado canônico exclusivamente via `GET /api/eu` (ou equivalente oficial) antes de testar qualquer nova ação.

---

## 2. 👥 REGRA DA TAXA COLETIVA EM GRUPO (COLLECTIVE THROTTLING)

Quando o bot operar em party/grupo cooperativo com compartilhamento de abates e bônus de EXP via WebSocket:
- A taxa de monstros por minuto de CADA membro individual deve ser reduzida proporcionalmente.
- **Fórmula Obrigatória:**
  $$\text{KillRate}_{\text{individual}} \le \frac{\text{KillRate}_{\text{solo\_seguro}}}{N_{\text{membros}} \times (1 + \text{BônusGrupo})}$$
- Use o helper nativo do template: `engine.CalculatePartyMaxKillRate(soloRate, partySize, bonusExpPercent)`.
- **Exemplo Real:** Se o teto seguro solo for 40 kills/min, em grupo de 4 com bônus de 45%, cada bot deve matar no máximo ~6.89 monstros/minuto (~8.7 segundos por monstro).

---

## 3. 🧩 PRESERVAÇÃO DE SCHEMA (`RawEstado map[string]any`)

- Nunca envie saves derivados apenas de structs parciais de Go.
- Guarde o JSON bruto recebido do servidor em `RawEstado map[string]any`.
- Altere apenas os campos estritamente necessários (ouro, level, exp) e reenvie o envelope completo `{ "estado": raw }`.
- Isso evita o erro de `"save malformado"` que ocorre quando o servidor espera dezenas de chaves adicionais (conquistas, gemas, skins, montaria, espolios, etc.).

---

## 4. 🔒 ISOLAMENTO DE INSTÂNCIAS E SESSÕES

- **`X-Instancia` Único:** Gere uma string única na inicialização (`Math.random() + Date.now()`) e envie em todas as requisições para evitar `outraAba: true`.
- **Multi-Contas com Multi-IP:** Quando rodar múltiplas contas na mesma VPS, configure IPs secundários (`bindIP: "10.0.0.X"`) usando `net.Dialer{LocalAddr: ...}` no HTTP e WebSocket.
- **Contas Distintas para Party:** Cada vaga no grupo deve ser uma conta cadastrada separada. Nunca conecte a mesma conta mais de uma vez.

---

## 5. 🛡️ SEGURANÇA DE DADOS, STEALTH E ZERO HARDCODING

1. **Zero Credenciais no Git:** NUNCA commite tokens, cookies, senhas, IPs de VPS ou arquivos `.key`. Use sempre `accounts.json` (no `.gitignore`) ou variáveis de ambiente.
2. **Nomes Anônimos no Dashboard Público:** NUNCA publique o nome real do personagem no GitHub Pages ou status público. Exiba sempre vocação/classe, índice da conta (`Conta #1`) e métricas técnicas.
3. **Consumo Leve na VPS:** Processo em Go puro, sem Chromium/Puppeteer na VPS, mantendo RAM abaixo de 15 MB por bot.
4. **Resiliência Total:** Toda conta deve rodar dentro de `engine.RunWithSupervisor` com `recover()`.
