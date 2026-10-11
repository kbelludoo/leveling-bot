package network

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type MessageHandler func(data []byte)
type DisconnectHandler func(err error)

// SocketClient gerencia conexão WebSocket com escrita thread-safe e keepalive
type SocketClient struct {
	url           string
	headers       http.Header
	proxyURL      string
	localAddr     string
	conn          *websocket.Conn
	writeMu       sync.Mutex
	mu            sync.RWMutex
	isConnected   bool
	onMessage     MessageHandler
	onDisconnect  DisconnectHandler
	closeChan     chan struct{}
}

func NewSocketClient(url string, headers http.Header, onMsg MessageHandler, onDisc DisconnectHandler) *SocketClient {
	return NewSocketClientWithNetwork(url, headers, "", "", onMsg, onDisc)
}

func NewSocketClientWithNetwork(url string, headers http.Header, proxyURL string, localAddr string, onMsg MessageHandler, onDisc DisconnectHandler) *SocketClient {
	if headers == nil {
		headers = make(http.Header)
	}
	// Headers padrão de navegador moderno
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	}

	return &SocketClient{
		url:          url,
		headers:      headers,
		proxyURL:     proxyURL,
		localAddr:    localAddr,
		onMessage:    onMsg,
		onDisconnect: onDisc,
		closeChan:    make(chan struct{}),
	}
}

func (s *SocketClient) Connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}

	// 1. Suporte a Proxy HTTP / SOCKS5
	if s.proxyURL != "" {
		parsedProxy, err := url.Parse(s.proxyURL)
		if err != nil {
			return fmt.Errorf("proxy URL inválida (%s): %w", s.proxyURL, err)
		}
		dialer.Proxy = http.ProxyURL(parsedProxy)
	}

	// 2. Suporte a bind de IP local dedicado (Multi-IP por placa de rede)
	if s.localAddr != "" {
		localTCPAddr, err := net.ResolveTCPAddr("tcp", s.localAddr+":0")
		if err != nil {
			return fmt.Errorf("endereço IP local inválido (%s): %w", s.localAddr, err)
		}
		netDialer := &net.Dialer{
			LocalAddr: localTCPAddr,
			Timeout:   15 * time.Second,
		}
		dialer.NetDialContext = netDialer.DialContext
	}

	conn, _, err := dialer.Dial(s.url, s.headers)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	s.mu.Lock()
	s.conn = conn
	s.isConnected = true
	s.closeChan = make(chan struct{})
	s.mu.Unlock()

	// Iniciar loop de leitura
	go s.readLoop()

	return nil
}

func (s *SocketClient) readLoop() {
	defer func() {
		s.Close()
	}()

	s.conn.SetReadLimit(10 * 1024 * 1024) // 10MB max packet

	// Configurar pong handler
	s.conn.SetPongHandler(func(string) error {
		_ = s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, message, err := s.conn.ReadMessage()
		if err != nil {
			s.mu.Lock()
			wasConnected := s.isConnected
			s.isConnected = false
			s.mu.Unlock()

			if wasConnected && s.onDisconnect != nil {
				s.onDisconnect(err)
			}
			return
		}

		if s.onMessage != nil {
			s.onMessage(message)
		}
	}
}

// SendJSON envia payload serializado em JSON com proteção de concorrência
func (s *SocketClient) SendJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.SendText(string(data))
}

// SendText envia string em modo TextMessage
func (s *SocketClient) SendText(text string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.RLock()
	conn := s.conn
	connected := s.isConnected
	s.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("socket não conectado")
	}

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, []byte(text))
}

// SendBinary envia bytes em modo BinaryMessage com proteção de concorrência
func (s *SocketClient) SendBinary(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.RLock()
	conn := s.conn
	connected := s.isConnected
	s.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("socket não conectado")
	}

	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

// SendPing envia frame de ping para keepalive
func (s *SocketClient) SendPing() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	s.mu.RLock()
	conn := s.conn
	connected := s.isConnected
	s.mu.RUnlock()

	if !connected || conn == nil {
		return nil
	}

	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteMessage(websocket.PingMessage, []byte{})
}

func (s *SocketClient) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isConnected
}

func (s *SocketClient) Close() {
	s.mu.Lock()
	if !s.isConnected {
		s.mu.Unlock()
		return
	}
	s.isConnected = false
	conn := s.conn
	s.conn = nil
	s.mu.Unlock()

	if conn != nil {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		_ = conn.Close()
	}
}
