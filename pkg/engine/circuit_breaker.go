package engine

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// CircuitState representa o estado atual do disjuntor de segurança anti-cheat
type CircuitState int

const (
	// StateClosed: Tudo normal. O bot caça e salva progresso livremente.
	StateClosed CircuitState = iota
	// StateOpen: Disjuntor desarmado. Erro de anti-cheat detectado. Combate parado e saves pausados.
	StateOpen
	// StateHalfOpen: Período de resfriamento terminou. Testando 1 requisição canário ou recarregando estado.
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED (Operação Normal)"
	case StateOpen:
		return "OPEN (Desarmado / Proteção Anti-Cheat Ativa)"
	case StateHalfOpen:
		return "HALF-OPEN (Testando Recuperação)"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig define as regras de disparo do disjuntor anti-ban
type CircuitBreakerConfig struct {
	// MaxFailures é a quantidade de recusas consecutivas antes de desarmar (recomendado: 1 ou 2)
	MaxFailures int
	// BaseCooldown é o tempo que o bot deve ficar parado esfriando (recomendado: 15m a 30m)
	BaseCooldown time.Duration
	// MaxCooldown é o teto máximo de espera com backoff exponencial (ex: 60m)
	MaxCooldown time.Duration
	// BackoffMultiplier multiplica o cooldown a cada novo desarme consecutivo
	BackoffMultiplier float64
	// OnTrip é executado imediatamente quando o disjuntor desarma (ex: desligar AutoHunt, recuar para safe zone)
	OnTrip func(reason string)
	// OnReset é executado quando o disjuntor retorna ao estado Closed
	OnReset func()
	// OnHalfOpen é executado quando o cooldown termina e o bot entra em verificação
	OnHalfOpen func()
}

// DefaultCircuitBreakerConfig retorna a configuração recomendada de proteção contra bans
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:       2,
		BaseCooldown:      15 * time.Minute,
		MaxCooldown:       60 * time.Minute,
		BackoffMultiplier: 2.0,
	}
}

// CircuitBreaker gerencia o disjuntor de segurança de uma conta ou processo
type CircuitBreaker struct {
	mu           sync.RWMutex
	name         string
	config       CircuitBreakerConfig
	state        CircuitState
	failures     int
	consecTrips  int
	lastFailure  string
	openedAt     time.Time
	currentWait  time.Duration
}

// NewCircuitBreaker cria uma nova instância de CircuitBreaker
func NewCircuitBreaker(name string, cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 2
	}
	if cfg.BaseCooldown <= 0 {
		cfg.BaseCooldown = 15 * time.Minute
	}
	if cfg.MaxCooldown <= 0 {
		cfg.MaxCooldown = 60 * time.Minute
	}
	if cfg.BackoffMultiplier <= 1.0 {
		cfg.BackoffMultiplier = 2.0
	}

	return &CircuitBreaker{
		name:        name,
		config:      cfg,
		state:       StateClosed,
		currentWait: cfg.BaseCooldown,
	}
}

// AllowAction informa se o bot tem permissão de realizar ações de jogo/salvamento no momento
func (cb *CircuitBreaker) AllowAction() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.checkStateTransitionLocked()
	return cb.state != StateOpen
}

// State retorna o estado atual (atualizando se o cooldown já expirou)
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.checkStateTransitionLocked()
	return cb.state
}

// StateString retorna uma string legível do estado atual
func (cb *CircuitBreaker) StateString() string {
	return cb.State().String()
}

// RecordSuccess registra que uma operação ou save foi aceito com sucesso pelo servidor
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.consecTrips = 0
	cb.currentWait = cb.config.BaseCooldown

	if cb.state != StateClosed {
		log.Printf("[%s] 🛡️ [CIRCUIT BREAKER] Servidor aceitou ação. Disjuntor restaurado para CLOSED.", cb.name)
		cb.state = StateClosed
		if cb.config.OnReset != nil {
			go cb.config.OnReset()
		}
	}
}

// RecordFailure registra uma falha ou recusa do servidor. Se atingir o limite, desarma imediatamente.
// Retorna true se o disjuntor foi desarmado nesta chamada.
func (cb *CircuitBreaker) RecordFailure(reason string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = reason

	log.Printf("[%s] ⚠️ [CIRCUIT BREAKER] Falha registrada (%d/%d): %s",
		cb.name, cb.failures, cb.config.MaxFailures, reason)

	if cb.failures >= cb.config.MaxFailures && cb.state != StateOpen {
		cb.tripLocked(reason)
		return true
	}

	return false
}

// Trip força o desarme imediato do disjuntor por um motivo crítico (ex: alerta de anti-cheat)
func (cb *CircuitBreaker) Trip(reason string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.tripLocked(reason)
}

// tripLocked assume que o mutex já está travado
func (cb *CircuitBreaker) tripLocked(reason string) {
	cb.state = StateOpen
	cb.openedAt = time.Now()
	cb.lastFailure = reason
	cb.consecTrips++

	// Calcula cooldown com backoff
	wait := cb.config.BaseCooldown
	for i := 1; i < cb.consecTrips; i++ {
		wait = time.Duration(float64(wait) * cb.config.BackoffMultiplier)
		if wait > cb.config.MaxCooldown {
			wait = cb.config.MaxCooldown
			break
		}
	}
	cb.currentWait = wait

	log.Printf("🚨 ======================================================================")
	log.Printf("🚨 [%s] [CIRCUIT BREAKER DESARMADO] MOTIVO: %s", cb.name, reason)
	log.Printf("🚨 Combate desarmado imediatamente. Saves suspensos por %v para evitar BAN.", cb.currentWait)
	log.Printf("🚨 ======================================================================")

	if cb.config.OnTrip != nil {
		go cb.config.OnTrip(reason)
	}
}

// Reset força o retorno manual ao estado Closed
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.consecTrips = 0
	cb.currentWait = cb.config.BaseCooldown
	cb.lastFailure = ""
}

// RemainingCooldown informa quanto tempo falta para o término do resfriamento
func (cb *CircuitBreaker) RemainingCooldown() time.Duration {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state != StateOpen {
		return 0
	}

	elapsed := time.Since(cb.openedAt)
	if elapsed >= cb.currentWait {
		return 0
	}
	return cb.currentWait - elapsed
}

// LastFailure retorna o último motivo de falha registrado
func (cb *CircuitBreaker) LastFailure() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastFailure
}

// checkStateTransitionLocked transiciona de Open para HalfOpen quando o cooldown vence
func (cb *CircuitBreaker) checkStateTransitionLocked() {
	if cb.state == StateOpen {
		if time.Since(cb.openedAt) >= cb.currentWait {
			cb.state = StateHalfOpen
			cb.failures = 0
			log.Printf("[%s] ⏱️ [CIRCUIT BREAKER] Cooldown expirado (%v). Entrando em modo HALF-OPEN para teste seguro.",
				cb.name, cb.currentWait)
			if cb.config.OnHalfOpen != nil {
				go cb.config.OnHalfOpen()
			}
		}
	}
}

// IsAntiCheatRejection analisa o código HTTP e a resposta do servidor para identificar sinais de anti-cheat ou rate limit
func IsAntiCheatRejection(statusCode int, responseBody string) (bool, string) {
	if statusCode == 429 {
		return true, "HTTP 429: Too Many Requests (Rate limit excedido pelo servidor)"
	}
	if statusCode == 403 && strings.Contains(strings.ToLower(responseBody), "cloudflare") {
		return true, "HTTP 403: Cloudflare / WAF Challenge detectado"
	}

	lower := strings.ToLower(responseBody)

	antiCheatKeywords := []struct {
		pattern string
		reason  string
	}{
		{"rápida demais", "Anti-Cheat: Acúmulo de experiência rápido demais para a margem de tempo do servidor"},
		{"rapida demais", "Anti-Cheat: Acúmulo de experiência rápido demais para a margem de tempo do servidor"},
		{"pontos de atributo demais", "Anti-Cheat: Violação de teto matemático de pontos de atributo"},
		{"contadores voltaram para trás", "Anti-Cheat: Quebra de monotonicidade (contadores voltaram para trás)"},
		{"contadores voltaram para tras", "Anti-Cheat: Quebra de monotonicidade (contadores voltaram para trás)"},
		{"relógio do jogo inválido", "Anti-Cheat: Descompasso de relógio entre cliente e servidor"},
		{"relogio do jogo invalido", "Anti-Cheat: Descompasso de relógio entre cliente e servidor"},
		{"save malformado", "Anti-Cheat: Schema de progresso incompleto ou corrompido"},
		{"\"recusado\":true", "Anti-Cheat: Servidor recusou a gravação do progresso explicitamente"},
		{"\"recusado\": true", "Anti-Cheat: Servidor recusou a gravação do progresso explicitamente"},
		{"\"outraaba\":true", "Anti-Cheat / Concorrência: Conflito de múltiplas instâncias ou abas simultâneas"},
		{"\"outraaba\": true", "Anti-Cheat / Concorrência: Conflito de múltiplas instâncias ou abas simultâneas"},
		{"\"banido\":true", "Suspensão: Conta banida ou suspensa pelo servidor"},
		{"\"banido\": true", "Suspensão: Conta banida ou suspensa pelo servidor"},
		{"programas de terceiros", "Suspensão: Detecção de bot/programa de terceiros pelo sistema do jogo"},
		{"muitas requisições", "Rate-Limit: Servidor solicitou desaceleração de requisições"},
		{"too many requests", "Rate-Limit: Excesso de requisições por minuto"},
	}

	for _, k := range antiCheatKeywords {
		if strings.Contains(lower, k.pattern) {
			return true, k.reason
		}
	}

	return false, ""
}

// CalculatePartyMaxKillRate calcula a taxa máxima segura de abates por minuto para cada membro individual em um grupo cooperativo.
// Garante que a soma da EXP compartilhada de todos os membros não ultrapasse o teto matemático do anti-cheat do servidor.
//
// Fórmula: TaxaIndividual <= TaxaMaxSolo / (TamanhoGrupo * (1 + BonusExpGrupo))
//
// Exemplo:
// Se o teto solo seguro for 40 monstros/minuto, em uma party de 4 com +45% de bônus (+0.45):
// 40 / (4 * 1.45) = 40 / 5.8 ~= 6.89 monstros/minuto por bot.
func CalculatePartyMaxKillRate(soloMaxKillRate float64, partySize int, groupBonusExpPercent float64) float64 {
	if partySize <= 1 {
		return soloMaxKillRate
	}

	multiplier := 1.0 + (groupBonusExpPercent / 100.0)
	totalPartyFactor := float64(partySize) * multiplier
	if totalPartyFactor <= 0 {
		return soloMaxKillRate
	}

	individualRate := soloMaxKillRate / totalPartyFactor
	return individualRate
}

// FormatSafePartySummary devolve um descritivo legível dos cálculos de party throttling
func FormatSafePartySummary(soloMaxKillRate float64, partySize int, groupBonusExpPercent float64) string {
	indivRate := CalculatePartyMaxKillRate(soloMaxKillRate, partySize, groupBonusExpPercent)
	return fmt.Sprintf("Party de %d bots com +%.0f%% bônus EXP: teto solo = %.1f/min -> teto seguro individual = %.2f/min (%.0fs por monstro)",
		partySize, groupBonusExpPercent, soloMaxKillRate, indivRate, 60.0/indivRate)
}
