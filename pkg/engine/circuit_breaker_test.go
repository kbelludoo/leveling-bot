package engine

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_NormalLifecycle(t *testing.T) {
	tripCalled := false
	resetCalled := false

	cfg := CircuitBreakerConfig{
		MaxFailures:       2,
		BaseCooldown:      50 * time.Millisecond,
		MaxCooldown:       200 * time.Millisecond,
		BackoffMultiplier: 2.0,
		OnTrip: func(reason string) {
			tripCalled = true
		},
		OnReset: func() {
			resetCalled = true
		},
	}

	cb := NewCircuitBreaker("test-acc", cfg)

	if !cb.AllowAction() {
		t.Fatalf("esperava AllowAction() == true no estado Closed")
	}
	if cb.State() != StateClosed {
		t.Fatalf("esperava StateClosed, obteve %v", cb.State())
	}

	// 1st failure - should not trip
	tripped := cb.RecordFailure("primeira falha leve")
	if tripped {
		t.Fatalf("não deveria ter desarmado na 1ª falha")
	}
	if !cb.AllowAction() {
		t.Fatalf("deveria continuar permitindo ações antes de bater o threshold")
	}

	// 2nd failure - should trip
	tripped = cb.RecordFailure("segunda falha (atingiu limite)")
	if !tripped {
		t.Fatalf("deveria ter desarmado na 2ª falha")
	}
	if cb.AllowAction() {
		t.Fatalf("não deveria permitir ações após o desarme")
	}
	if cb.State() != StateOpen {
		t.Fatalf("esperava StateOpen após atingir o limite")
	}

	time.Sleep(10 * time.Millisecond)
	if !tripCalled {
		t.Fatalf("esperava que o callback OnTrip tivesse sido invocado")
	}

	// Wait for cooldown to expire
	time.Sleep(60 * time.Millisecond)

	// Now it should transition to StateHalfOpen
	if !cb.AllowAction() {
		t.Fatalf("esperava AllowAction() == true em HalfOpen para teste")
	}
	if cb.State() != StateHalfOpen {
		t.Fatalf("esperava StateHalfOpen após o cooldown")
	}

	// Canary succeeds -> back to Closed
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Fatalf("esperava StateClosed após RecordSuccess()")
	}

	time.Sleep(10 * time.Millisecond)
	if !resetCalled {
		t.Fatalf("esperava que o callback OnReset tivesse sido invocado")
	}
}

func TestCircuitBreaker_ImmediateTrip(t *testing.T) {
	cb := NewCircuitBreaker("test-immediate", CircuitBreakerConfig{
		MaxFailures:  5,
		BaseCooldown: 10 * time.Minute,
	})

	cb.Trip("Anti-Cheat detectou anomalia crítica")

	if cb.AllowAction() {
		t.Fatalf("deveria estar bloqueado após Trip imediato")
	}
	if cb.State() != StateOpen {
		t.Fatalf("esperava StateOpen")
	}
	if cb.LastFailure() != "Anti-Cheat detectou anomalia crítica" {
		t.Fatalf("motivo inesperado: %s", cb.LastFailure())
	}
	if cb.RemainingCooldown() <= 0 {
		t.Fatalf("RemainingCooldown deveria ser > 0")
	}
}

func TestIsAntiCheatRejection(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantReject bool
	}{
		{
			name:       "Rate limit 429",
			statusCode: 429,
			body:       `{"erro": "Too Many Requests"}`,
			wantReject: true,
		},
		{
			name:       "LoL Idle XP rápida demais",
			statusCode: 200,
			body:       `{"erro": "experiência rápida demais para este personagem (38273 > 18272)", "recusado": true}`,
			wantReject: true,
		},
		{
			name:       "Contadores voltaram para tras",
			statusCode: 200,
			body:       `{"erro": "contadores voltaram para trás"}`,
			wantReject: true,
		},
		{
			name:       "Pontos de atributo demais",
			statusCode: 200,
			body:       `{"erro": "pontos de atributo demais"}`,
			wantReject: true,
		},
		{
			name:       "Outra aba ativa",
			statusCode: 200,
			body:       `{"outraAba": true}`,
			wantReject: true,
		},
		{
			name:       "Conta banida",
			statusCode: 200,
			body:       `{"erro": "Esta conta está suspensa até 24/10/2026. Motivo: uso de programas de terceiros.", "banido": true}`,
			wantReject: true,
		},
		{
			name:       "Resposta normal de sucesso",
			statusCode: 200,
			body:       `{"ok": true, "rev": 4500, "ouro": 1200}`,
			wantReject: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReject, reason := IsAntiCheatRejection(tt.statusCode, tt.body)
			if gotReject != tt.wantReject {
				t.Errorf("IsAntiCheatRejection() = %v (motivo: %s), esperava %v", gotReject, reason, tt.wantReject)
			}
		})
	}
}

func TestCalculatePartyMaxKillRate(t *testing.T) {
	// Solo: deve retornar a taxa solo original
	soloRate := CalculatePartyMaxKillRate(40.0, 1, 45.0)
	if soloRate != 40.0 {
		t.Fatalf("esperava 40.0 para party de 1, obteve %f", soloRate)
	}

	// 4 players com +45% de bonus
	// Total factor = 4 * 1.45 = 5.8
	// Rate individual = 40.0 / 5.8 ~= 6.8965
	partyRate := CalculatePartyMaxKillRate(40.0, 4, 45.0)
	if partyRate < 6.89 || partyRate > 6.90 {
		t.Fatalf("esperava rate por volta de 6.896, obteve %f", partyRate)
	}

	// Verifica se a taxa combinada não estoura o solo seguro
	combinedExpMultiplier := partyRate * 4.0 * 1.45
	if combinedExpMultiplier > 40.01 || combinedExpMultiplier < 39.99 {
		t.Fatalf("invariante violada: taxa combinada deveria ser exatamente igual a solo (40.0), obteve %f", combinedExpMultiplier)
	}
}

func TestCircuitBreaker_Concurrency(t *testing.T) {
	cb := NewCircuitBreaker("concurrent-test", CircuitBreakerConfig{
		MaxFailures:  100,
		BaseCooldown: 50 * time.Millisecond,
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			cb.AllowAction()
			cb.RecordSuccess()
		}()
		go func(idx int) {
			defer wg.Done()
			cb.RecordFailure("falha concorrente")
		}(i)
	}
	wg.Wait()
}
