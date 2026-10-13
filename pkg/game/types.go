package game

// AccountConfig modelo de configuração lido do accounts.json
type AccountConfig struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Enabled     bool            `json:"enabled"`
	Proxy       string          `json:"proxy,omitempty"`    // Suporte Multi-IP via Proxy HTTP/SOCKS5 (ex: http://user:pass@host:port)
	LocalIP     string          `json:"localIp,omitempty"`  // Suporte Multi-IP via interface de rede local (ex: 192.168.3.205)
	Credentials GameCredentials `json:"credentials"`
	Settings    GameSettings    `json:"settings"`
}

type GameCredentials struct {
	Username      string `json:"username"`
	Email         string `json:"email"`
	Password      string `json:"password"`
	SessionCookie string `json:"sessionCookie"`
	Token         string `json:"token"`
	DeviceUUID    string `json:"deviceUuid"`
}

type GameSettings struct {
	HuntTarget   string   `json:"huntTarget"`
	AutoHunt     bool     `json:"autoHunt"`
	AutoSell     bool     `json:"autoSell"`
	AutoHeal     bool     `json:"autoHeal"`
	AutoDaily    bool     `json:"autoDaily"`
	Lure         int      `json:"lure"`
	PartyLeader  bool     `json:"partyLeader,omitempty"`  // Se true, este bot cria a party e convida os membros
	PartyMembers []string `json:"partyMembers,omitempty"` // Lista de usuários para convidar (usado pelo líder)
	AutoParty    bool     `json:"autoParty,omitempty"`    // Se true, aceita convites de party automaticamente
	CodexMode    bool     `json:"codexMode,omitempty"`    // Se true, ativa o Modo Completista do Bestiário (Codex)
	CodexTarget  uint64   `json:"codexTarget,omitempty"`  // Meta de abates por monstro (padrão: 2000 ou 5000)
}

// Mensagens básicas de controle de caçada e status
type GameInboundPacket struct {
	Opcode string `json:"op"`
	Data   any    `json:"data"`
}

type GameOutboundPacket struct {
	Opcode string `json:"op"`
	Data   any    `json:"data"`
}
