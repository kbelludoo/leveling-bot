package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"bot-template-go/pkg/engine"
	"bot-template-go/pkg/game"
	"bot-template-go/pkg/server"
)

//go:embed accounts.example.json
var defaultAccountsJSON []byte

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000.0)
	}
	return fmt.Sprintf("%.2fkk", float64(n)/1_000_000.0)
}

func main() {
	log.Println("================================================================================")
	log.Println("       ⚡ NATIVE GO MULTI-BOT ENGINE (ULTRA-LEVE & ALTA PERFORMANCE) ⚡        ")
	log.Println("================================================================================")

	// 1. Carregar contas do accounts.json (cria a partir do template se não existir)
	configFile := "accounts.json"
	content, err := os.ReadFile(configFile)
	if err != nil {
		log.Printf("Aviso: %s não encontrado. Criando arquivo a partir do template...", configFile)
		_ = os.WriteFile(configFile, defaultAccountsJSON, 0644)
		content = defaultAccountsJSON
	}

	var accountConfigs []game.AccountConfig
	if err := json.Unmarshal(content, &accountConfigs); err != nil {
		log.Fatalf("Falha ao analisar %s: %v", configFile, err)
	}

	targetAcc := os.Getenv("ACCOUNT")
	var activeConfigs []game.AccountConfig

	for _, a := range accountConfigs {
		if !a.Enabled && a.ID != targetAcc {
			continue
		}
		if targetAcc != "" && a.ID != targetAcc {
			continue
		}
		activeConfigs = append(activeConfigs, a)
	}

	if len(activeConfigs) == 0 {
		log.Println("⚠️ Nenhuma conta com 'enabled: true' encontrada em accounts.json.")
		log.Println("Edite accounts.json e habilite sua conta para iniciar.")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := engine.NewBotManager()

	log.Printf("Iniciando %d conta(s) em Goroutines isoladas com supervisor…\n", len(activeConfigs))
	for _, cfg := range activeConfigs {
		b := game.NewBot(cfg)
		manager.Register(b)
		go engine.RunWithSupervisor(ctx, b)
	}

	// 2. Iniciar Servidor Web / Dashboard HTTP
	webPort := 3003
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			webPort = v
		}
	}

	srv := server.NewServer(webPort, manager)
	go func() {
		log.Printf("[Web Server] Dashboard online em http://localhost:%d\n", webPort)
		if err := srv.Start(); err != nil {
			log.Printf("[Web Server] Encerrado: %v", err)
		}
	}()

	// 3. Monitor no Terminal (atualiza a cada 4 segundos com estatísticas de RAM)
	go func() {
		time.Sleep(2 * time.Second)
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				allocMB := float64(m.Alloc) / 1024.0 / 1024.0
				sysMB := float64(m.Sys) / 1024.0 / 1024.0

				statuses := manager.GetAllStatuses()
				var totalGold int64 = 0
				totalKills := 0
				huntingCount := 0

				for _, st := range statuses {
					totalGold += st.Gold
					totalKills += st.Kills
					if st.Status == "hunting" {
						huntingCount++
					}
				}

				fmt.Print("\033[H\033[2J") // Limpar tela do terminal
				fmt.Println("================================================================================")
				fmt.Printf("   ⚡ BOT ENGINE CONSOLE — RAM: %.2f MB (Sys: %.2f MB) | Goroutines: %d\n", allocMB, sysMB, runtime.NumGoroutine())
				fmt.Printf("   📊 Contas: %d/%d Caçando | Ouro: %s | Total Kills: %d\n", huntingCount, len(statuses), formatNumber(totalGold), totalKills)
				fmt.Printf("   🌐 Dashboard Web: http://localhost:%d\n", webPort)
				fmt.Println("================================================================================")

				for _, st := range statuses {
					hpPct := 0
					if st.MaxHP > 0 {
						hpPct = (st.HP * 100) / st.MaxHP
					}
					manaPct := 0
					if st.MaxMana > 0 {
						manaPct = (st.Mana * 100) / st.MaxMana
					}

					fmt.Printf("▸ [%s] %s (Lv. %d %s) | Status: %s\n", st.ID, st.Name, st.Level, st.Vocation, st.Status)
					fmt.Printf("  HP: %d/%d (%d%%) | Mana: %d/%d (%d%%) | Ouro: %s | Kills: %d\n", st.HP, st.MaxHP, hpPct, st.Mana, st.MaxMana, manaPct, formatNumber(st.Gold), st.Kills)
					if st.CurrentArea != "" {
						fmt.Printf("  Área: %s | EXP/H: %.0f | Duração: %ds\n", st.CurrentArea, st.ExpPerHour, st.DurationSec)
					}
					fmt.Println("--------------------------------------------------------------------------------")
				}
			}
		}
	}()

	// 4. Capturar sinais para desligamento limpo
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("\nSinal de encerramento recebido. Desligando bots...")
	cancel()
	time.Sleep(1 * time.Second)
	log.Println("Bot encerrado com sucesso.")
}
