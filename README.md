# 🏹 Leveling Idle 2D Bot (Go Native Engine)

Bot autônomo nativo de altíssima performance desenvolvido em **Go (Golang)** para o jogo **Leveling Idle 2D** ([levelingidle2d.com.br](https://levelingidle2d.com.br/jogar)), com comunicação direta via **WebSocket Binário Protobuf 3** e **Dashboard ao vivo no GitHub Pages**.

🌐 **Dashboard ao Vivo (GitHub Pages):** [https://kbelludoo.github.io/leveling-bot/](https://kbelludoo.github.io/leveling-bot/)

---

## 🌟 Recursos Principais

- ⚡ **Comunicação Direta WebSocket Protobuf 3:** Conecta em `wss://levelingidle2d.com.br/ws` com handshakes e pacotes binários nativos (`leveling.v1`). Zero navegadores headless (Puppeteer/Selenium).
- 🪶 **Consumo Cirúrgico de Recursos:** Consome apenas **~2.1 MB de RAM** e quase 0% de CPU.
- 🏹 **Otimizado para Arqueiro (Rank S+ Meta):** Configurado para a classe mais forte do jogo, aproveitando alcance de tela inteira (zero tempo de viagem) e alta cadência de ataque.
- 🛡️ **Supervisão Contínua:** Goroutines protegidas com `engine.RunWithSupervisor` e auto-reconexão contra oscilações de rede.
- 🌐 **Web Dashboard Embutido:** Servidor HTTP local independente na porta **3003** (`/api/status` e `/api/action`).
- 📺 **Console Ticker no Terminal:** Estatísticas em tempo real de HP, Mana, EXP/h, Kills, Ouro e Área de caçada atualizadas a cada 4 segundos.
- 🚀 **Pronto para VPS Linux:** Script de compilação cruzada e arquivo de serviço systemd incluídos.

---

## 📁 Estrutura do Projeto

```text
leveling-bot/
├── accounts.json               # Credenciais reais (IGNORADO PELO GIT POR SEGURANÇA)
├── accounts.example.json       # Modelo de configuração de contas
├── main.go                     # Entrypoint universal (Porta 3003)
├── go.mod / go.sum             # Módulo Go nativo (gorilla/websocket + protobuf)
│
├── docs/                       # 🌐 GITHUB PAGES (DASHBOARD AO VIVO)
│   ├── index.html              # Interface Cyberpunk / Dark com status dos bots
│   └── status.json             # Telemetria em tempo real
│
├── pkg/
│   ├── engine/                 # Motor: ciclo de vida, supervisor e gerenciador
│   ├── network/                # Conexões: WebSocket thread-safe (SendBinary) e HTTP
│   ├── game/                   # 🎮 Lógica do Leveling Idle: login protobuf, caçada e stats
│   └── server/                 # Servidor HTTP local e Dashboard embutido (Porta 3003)
│
└── scripts/
    ├── build-linux.bat         # Compila binário Linux para VPS
    └── bot.service             # Configuração para systemd na VPS Ubuntu
```

---

## 🚀 Como Executar Localmente

1. **Configuração de Contas:**
   Copie `accounts.example.json` para `accounts.json` e informe suas credenciais:
   ```json
   [
     {
       "id": "acc-1",
       "name": "kbelludoo",
       "enabled": true,
       "credentials": {
         "username": "kbelludoo",
         "password": "sua_senha"
       },
       "settings": {
         "huntTarget": "floresta-sombria",
         "autoHunt": true,
         "autoSell": true,
         "autoHeal": true
       }
     }
   ]
   ```

2. **Executar:**
   ```bash
   go run .
   ```

3. **Acessar o Dashboard Local:**
   Abra [http://localhost:3003](http://localhost:3003) no seu navegador.

---

## 📦 Compilação para VPS Linux

No Windows:
```cmd
cmd /c "set GOOS=linux&& set GOARCH=amd64&& go build -ldflags=\"-s -w\" -o bot-leveling-linux ."
```

Envie para a VPS:
```bash
scp bot-leveling-linux ubuntu@SEU_IP_DA_VPS:/home/ubuntu/leveling-bot/
```
