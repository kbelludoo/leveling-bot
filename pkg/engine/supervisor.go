package engine

import (
	"context"
	"log"
	"time"
)

// RunWithSupervisor roda um bot continuamente, reiniciando com backoff se houver queda ou panic
func RunWithSupervisor(ctx context.Context, b GameBot) {
	backoff := 3 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Printf("[%s] Supervisor encerrando bot...", b.ID())
			return
		default:
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[%s] ⚠️ CRASH PREVENIDO (Panic recuperado): %v. Reiniciando em %v...", b.ID(), r, backoff)
					time.Sleep(backoff)
					backoff = min(backoff*2, maxBackoff)
				}
			}()

			log.Printf("[%s] Inicializando bot...", b.ID())
			if err := b.Init(ctx); err != nil {
				log.Printf("[%s] Falha no Init: %v. Tentando novamente em %v...", b.ID(), err, backoff)
				time.Sleep(backoff)
				backoff = min(backoff*2, maxBackoff)
				return
			}

			// Se o Init foi bem sucedido, resetamos o backoff
			backoff = 3 * time.Second

			log.Printf("[%s] Conexão estabelecida! Entrando no loop principal...", b.ID())
			if err := b.Run(ctx); err != nil {
				log.Printf("[%s] Conexão encerrada: %v. Reconectando em %v...", b.ID(), err, backoff)
				time.Sleep(backoff)
				backoff = min(backoff*2, maxBackoff)
			}
		}()
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
