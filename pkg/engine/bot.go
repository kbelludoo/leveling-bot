package engine

import (
	"context"
	"time"
)

// GameBot define o ciclo de vida obrigatório que todo bot de jogo deve satisfazer.
type GameBot interface {
	// ID identificador único da conta (ex: acc-1)
	ID() string

	// Name apelido do personagem ou jogador
	Name() string

	// Init valida credenciais, realiza login HTTP ou obtém tokens iniciais
	Init(ctx context.Context) error

	// Run executa o loop contínuo do jogo (WebSocket ou Polling HTTP). Deve bloquear até ctx ser cancelado ou ocorrer erro fatal.
	Run(ctx context.Context) error

	// GetStatus devolve uma cópia thread-safe dos status atuais do personagem/bot
	GetStatus() AccountStatus

	// ExecuteAction permite disparar comandos manuais via Web Dashboard (ex: "heal", "travel", "autoHunt")
	ExecuteAction(action string, payload map[string]any) (any, error)

	// SupplyTicket entrega um ticket ou token externo de sessão (ex: via bypass CDP ou POST manual)
	SupplyTicket(ticket, wsURL string) error
}

// AccountStatus estrutura padronizada para exibição no Terminal e na API HTTP/Web Dashboard
type AccountStatus struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Level       int            `json:"level"`
	Vocation    string         `json:"vocation,omitempty"`
	HP          int            `json:"hp"`
	MaxHP       int            `json:"maxHp"`
	Mana        int            `json:"mana"`
	MaxMana     int            `json:"maxMana"`
	Gold        int64          `json:"gold"`
	Kills       int            `json:"kills"`
	ExpPerHour  float64        `json:"expPerHour"`
	Status      string         `json:"status"` // "hunting", "training", "idle", "reconnecting", "offline"
	CurrentArea string         `json:"currentArea"`
	DurationSec int            `json:"durationSec"`
	Logs        []string       `json:"logs"`
	UpdatedAt   int64          `json:"updatedAt"`
	Extra       map[string]any `json:"extra,omitempty"` // Campos extras específicos do jogo (ex: pokemons, charms, stamina)
}

// NewStatus inicializa um status limpo
func NewStatus(id, name string) AccountStatus {
	return AccountStatus{
		ID:        id,
		Name:      name,
		Status:    "Iniciando",
		UpdatedAt: time.Now().UnixMilli(),
		Logs:      make([]string, 0),
		Extra:     make(map[string]any),
	}
}
