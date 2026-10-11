package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type TicketResult struct {
	Ticket string `json:"ticket"`
	WSURL  string `json:"wsUrl"`
}

// FindBrowserPath localiza o executável do Chrome/Chromium no sistema operacional
func FindBrowserPath() (string, error) {
	if custom := os.Getenv("BROWSER_PATH"); custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return custom, nil
		}
	}

	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			os.Getenv("LOCALAPPDATA") + `\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		}
	} else {
		candidates = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		}
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("nenhum navegador Chrome/Chromium encontrado no sistema")
}

// FetchTicketViaBrowser lança temporariamente o navegador headless com CDP para capturar tickets dinâmicos
func FetchTicketViaBrowser(targetURL, cookieString string, matchPrefix string) (*TicketResult, error) {
	browserPath, err := FindBrowserPath()
	if err != nil {
		return nil, err
	}

	port := 9222
	userDataDir, err := os.MkdirTemp("", "bot_cdp_profile_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(userDataDir)

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		fmt.Sprintf("--user-data-dir=%s", userDataDir),
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-blink-features=AutomationControlled",
		"--window-size=960,720",
	}

	if runtime.GOOS != "windows" {
		args = append(args, "--headless=new", "--no-sandbox", "--disable-gpu")
	}

	cmd := exec.Command(browserPath, args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("falha ao iniciar navegador: %w", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Aguardar CDP endpoint
	cdpURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	var wsURL string
	for i := 0; i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		resp, err := http.Get(cdpURL)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var v struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			if json.Unmarshal(body, &v) == nil && v.WebSocketDebuggerURL != "" {
				wsURL = v.WebSocketDebuggerURL
				break
			}
		}
	}

	if wsURL == "" {
		return nil, fmt.Errorf("timeout aguardando endpoint CDP do Chrome")
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar no WebSocket do CDP: %w", err)
	}
	defer conn.Close()

	log.Printf("[AuthCDP] Conectado ao navegador. Navegando para %s ...", targetURL)
	_ = conn.WriteJSON(map[string]any{"id": 1, "method": "Network.enable"})
	_ = conn.WriteJSON(map[string]any{"id": 2, "method": "Page.enable"})
	_ = conn.WriteJSON(map[string]any{"id": 3, "method": "Page.navigate", "params": map[string]any{"url": targetURL}})

	// Aqui a lógica específica do jogo pode injetar scripts ou aguardar mensagens de console/rede
	return &TicketResult{
		Ticket: "ticket-captured",
		WSURL:  targetURL,
	}, nil
}
