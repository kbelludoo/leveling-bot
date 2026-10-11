package game

// AccountConfig modelo de configuração lido do accounts.json
type AccountConfig struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Enabled     bool            `json:"enabled"`
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
	HuntTarget string `json:"huntTarget"`
	AutoHunt   bool   `json:"autoHunt"`
	AutoSell   bool   `json:"autoSell"`
	AutoHeal   bool   `json:"autoHeal"`
	AutoDaily  bool   `json:"autoDaily"`
	Lure       int    `json:"lure"`
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
