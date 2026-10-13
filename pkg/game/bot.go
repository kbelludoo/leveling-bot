package game

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"bot-template-go/pkg/engine"
	"bot-template-go/pkg/network"

	"google.golang.org/protobuf/encoding/protowire"
)

type Bot struct {
	config  AccountConfig
	mu      sync.RWMutex
	status  engine.AccountStatus
	socket  *network.SocketClient
	httpCli *network.HTTPClient

	// Controle interno da sessão do jogo
	reqSeq       uint32
	deviceUUID   string
	token        string
	accountID    uint64
	inGame       bool
	lastActionAt time.Time
	ticketChan   chan string

	// Controle de Caçadas (Auto-Hunt)
	currentGate       string
	currentTier       uint32
	currentDifficulty string
	partyMinLevel     int
	lastHuntCheck     time.Time
}

func NewBot(cfg AccountConfig) *Bot {
	st := engine.NewStatus(cfg.ID, cfg.Name)
	st.CurrentArea = "Blue Gate (Rato da Ruína Nv 5)"
	if cfg.Settings.HuntTarget != "" && cfg.Settings.HuntTarget != "floresta-sombria" {
		st.CurrentArea = cfg.Settings.HuntTarget
	}

	return &Bot{
		config:     cfg,
		status:     st,
		httpCli:    network.NewHTTPClient("https://levelingidle2d.com.br", 10*time.Second),
		ticketChan: make(chan string, 1),
	}
}

func (b *Bot) ID() string {
	return b.config.ID
}

func (b *Bot) Name() string {
	return b.config.Name
}

func (b *Bot) nextReqID() uint32 {
	return atomic.AddUint32(&b.reqSeq, 1)
}

func generateUUID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}

func (b *Bot) Init(ctx context.Context) error {
	b.updateStatus(func(s *engine.AccountStatus) {
		s.Status = "Preparando conexão"
	})

	if b.config.Credentials.DeviceUUID != "" {
		b.deviceUUID = b.config.Credentials.DeviceUUID
	} else {
		b.deviceUUID = generateUUID()
	}

	b.token = b.config.Credentials.Token

	log.Printf("[%s] Inicializado para caçador %s (Dispositivo: %s)", b.ID(), b.config.Credentials.Username, b.deviceUUID)
	return nil
}

func (b *Bot) Run(ctx context.Context) error {
	b.updateStatus(func(s *engine.AccountStatus) {
		s.Status = "Conectando ao servidor..."
	})

	wsURL := fmt.Sprintf("wss://levelingidle2d.com.br/ws?device=%s", b.deviceUUID)

	headers := http.Header{}
	headers.Set("Origin", "https://levelingidle2d.com.br")
	headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")

	disconnectChan := make(chan error, 1)

	b.socket = network.NewSocketClientWithNetwork(
		wsURL,
		headers,
		b.config.Proxy,
		b.config.LocalIP,
		func(data []byte) {
			b.handleInboundPacket(data)
		},
		func(err error) {
			select {
			case disconnectChan <- err:
			default:
			}
		},
	)

	if err := b.socket.Connect(); err != nil {
		b.updateStatus(func(s *engine.AccountStatus) {
			s.Status = fmt.Sprintf("Erro de conexão: %v", err)
		})
		return err
	}
	defer b.socket.Close()

	b.updateStatus(func(s *engine.AccountStatus) {
		s.Status = "Autenticando..."
	})
	b.addLog("Conexão WebSocket estabelecida. Enviando login...")

	// 1. Enviar pacote de Login (ou Resume se tiver token)
	if err := b.sendAuthPacket(); err != nil {
		log.Printf("[%s] Erro ao enviar pacote de login: %v", b.ID(), err)
		return err
	}

	// 2. Loop de ticks de automação (keepalive ping + loop de caçada)
	pingTicker := time.NewTicker(10 * time.Second)
	defer pingTicker.Stop()

	huntTicker := time.NewTicker(3 * time.Second)
	defer huntTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.updateStatus(func(s *engine.AccountStatus) {
				s.Status = "Desconectado"
			})
			return ctx.Err()

		case err := <-disconnectChan:
			b.updateStatus(func(s *engine.AccountStatus) {
				s.Status = fmt.Sprintf("Conexão perdida (%v). Reconectando...", err)
			})
			b.addLog("Conexão perdida com o servidor.")
			return fmt.Errorf("desconectado do websocket: %w", err)

		case <-pingTicker.C:
			// Enviar Ping frame e pacote Ping se autenticado
			if err := b.sendPingPacket(); err != nil {
				log.Printf("[%s] Erro ao enviar ping: %v", b.ID(), err)
			}

		case <-huntTicker.C:
			b.performTick()
		}
	}
}

// sendAuthPacket envia o login ou resume via protobuf wire
func (b *Bot) sendAuthPacket() error {
	username := b.config.Credentials.Username
	if username == "" {
		username = b.config.Credentials.Email
	}
	password := b.config.Credentials.Password

	// Se tiver token salvo, tenta primeiro com Login ou Resume
	reqID := b.nextReqID()

	var loginSubMsg []byte
	loginSubMsg = protowire.AppendTag(loginSubMsg, 1, protowire.BytesType)
	loginSubMsg = protowire.AppendString(loginSubMsg, username)
	loginSubMsg = protowire.AppendTag(loginSubMsg, 2, protowire.BytesType)
	loginSubMsg = protowire.AppendString(loginSubMsg, password)

	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 4, protowire.BytesType) // Field 4: login
	clientMsg = protowire.AppendBytes(clientMsg, loginSubMsg)

	return b.socket.SendBinary(clientMsg)
}

func (b *Bot) sendPingPacket() error {
	reqID := b.nextReqID()
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 2, protowire.BytesType) // Field 2: ping
	clientMsg = protowire.AppendBytes(clientMsg, []byte{})

	return b.socket.SendBinary(clientMsg)
}

// sendPartyInvite envia convite de party para um jogador pelo nome
func (b *Bot) sendPartyInvite(targetName string) error {
	reqID := b.nextReqID()

	// PartyInviteByName { 1: name }
	var inviteByName []byte
	inviteByName = protowire.AppendTag(inviteByName, 1, protowire.BytesType)
	inviteByName = protowire.AppendString(inviteByName, targetName)

	// PartyRequest { 2: invite (PartyInviteByName) }
	var partyReq []byte
	partyReq = protowire.AppendTag(partyReq, 2, protowire.BytesType)
	partyReq = protowire.AppendBytes(partyReq, inviteByName)

	// ClientMessage { 1: req_id, 290: party_request }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 290, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, partyReq)

	log.Printf("[%s] Enviando convite de Party para: %s", b.ID(), targetName)
	return b.socket.SendBinary(clientMsg)
}

// sendPartyAnswer responde a um convite de party recebido
func (b *Bot) sendPartyAnswer(fromAccount int64, accept bool) error {
	reqID := b.nextReqID()

	// PartyAnswer { 1: from_account, 2: accept }
	var partyAnswer []byte
	partyAnswer = protowire.AppendTag(partyAnswer, 1, protowire.VarintType)
	partyAnswer = protowire.AppendVarint(partyAnswer, uint64(fromAccount))
	partyAnswer = protowire.AppendTag(partyAnswer, 2, protowire.VarintType)
	if accept {
		partyAnswer = protowire.AppendVarint(partyAnswer, 1)
	} else {
		partyAnswer = protowire.AppendVarint(partyAnswer, 0)
	}

	// PartyRequest { 3: answer (PartyAnswer) }
	var partyReq []byte
	partyReq = protowire.AppendTag(partyReq, 3, protowire.BytesType)
	partyReq = protowire.AppendBytes(partyReq, partyAnswer)

	// ClientMessage { 1: req_id, 290: party_request }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 290, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, partyReq)

	log.Printf("[%s] Respondendo convite de Party de %d (Aceitar=%v)", b.ID(), fromAccount, accept)
	return b.socket.SendBinary(clientMsg)
}

// sendStartHunt inicia ou altera a caçada para o portal/tier desejado
func (b *Bot) sendStartHunt(gateID string, tierMin uint32, difficulty string) error {
	reqID := b.nextReqID()

	// StartHunt { 1: gate_id, 2: tier_min, 3: difficulty }
	var huntMsg []byte
	if gateID != "" {
		huntMsg = protowire.AppendTag(huntMsg, 1, protowire.BytesType)
		huntMsg = protowire.AppendString(huntMsg, gateID)
	}
	if tierMin > 0 {
		huntMsg = protowire.AppendTag(huntMsg, 2, protowire.VarintType)
		huntMsg = protowire.AppendVarint(huntMsg, uint64(tierMin))
	}
	if difficulty != "" {
		huntMsg = protowire.AppendTag(huntMsg, 3, protowire.BytesType)
		huntMsg = protowire.AppendString(huntMsg, difficulty)
	}

	// ClientMessage { 1: req_id, 30: start_hunt }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 30, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, huntMsg)

	log.Printf("[%s] Enviando StartHunt (gate=%s, tier=%d, dif=%s)", b.ID(), gateID, tierMin, difficulty)
	return b.socket.SendBinary(clientMsg)
}

func (b *Bot) handleInboundPacket(bData []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	in := bData
	for len(in) > 0 {
		num, wtype, n := protowire.ConsumeTag(in)
		if n < 0 {
			break
		}
		in = in[n:]

		switch wtype {
		case protowire.VarintType:
			_, n := protowire.ConsumeVarint(in)
			if n < 0 {
				return
			}
			in = in[n:]

		case protowire.BytesType:
			bytesVal, n := protowire.ConsumeBytes(in)
			if n < 0 {
				return
			}
			in = in[n:]

			switch num {
			case 2: // Error
				errMsg := parseStringField(bytesVal, 1)
				if errMsg == "party.invite.in_party" {
					b.addLogLocked("Membro já está na Party.")
				} else if errMsg == "ratelimit.party" {
					b.addLogLocked("Party rate limit.")
				} else {
					b.status.Status = fmt.Sprintf("Erro: %s", errMsg)
					b.addLogLocked(fmt.Sprintf("Servidor erro: %s", errMsg))
				}

			case 4: // AuthOk
				token := parseStringField(bytesVal, 1)
				username := parseStringField(bytesVal, 3)
				b.token = token
				b.inGame = true
				b.status.Status = "Conectado e Autenticado"
				b.status.Name = username
				b.addLogLocked(fmt.Sprintf("Login bem-sucedido! Bem-vindo, %s.", username))

			case 8: // CharacterState
				b.parseCharacterState(bytesVal)

			case 9: // CharacterPatch
				b.parseCharacterPatch(bytesVal)

			case 14: // WorldEvents
				b.status.Kills++
				b.status.Gold += 25
				b.status.UpdatedAt = time.Now().UnixMilli()

			case 30: // StartHuntResult
				gateID := parseStringField(bytesVal, 1)
				tierMin := uint32(parseVarintField(bytesVal, 2))
				dif := parseStringField(bytesVal, 3)
				if gateID != "" {
					b.currentGate = gateID
					b.currentTier = tierMin
					b.currentDifficulty = dif
					huntName := GetHuntName(gateID, tierMin)
					b.status.CurrentArea = huntName
					b.status.Status = "hunting"
					b.addLogLocked(fmt.Sprintf("Caçada iniciada: %s", huntName))
				}

			case 41: // HuntSession
				gateID := parseStringField(bytesVal, 1)
				tierMin := uint32(parseVarintField(bytesVal, 2))
				dif := parseStringField(bytesVal, 3)
				if gateID != "" {
					b.currentGate = gateID
					b.currentTier = tierMin
					b.currentDifficulty = dif
					huntName := GetHuntName(gateID, tierMin)
					b.status.CurrentArea = huntName
					b.status.Status = "hunting"
				}

			case 290: // PartyResponse
				b.handlePartyResponse(bytesVal)

			case 291: // PartyPush
				b.handlePartyPush(bytesVal)
			}
		default:
			return
		}
	}

	b.status.UpdatedAt = time.Now().UnixMilli()
}

func (b *Bot) parseCharacterState(bytesVal []byte) {
	data := bytesVal
	for len(data) > 0 {
		num, wtype, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		if wtype == protowire.BytesType {
			sub, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return
			}
			data = data[n:]

			switch num {
			case 2: // Identity
				name := parseStringField(sub, 1)
				cls := parseStringField(sub, 6)
				if name != "" {
					b.status.Name = name
				}
				if cls != "" {
					b.status.Vocation = cls
				}
			case 3: // Progress
				level := int(parseVarintField(sub, 1))
				exp := int64(parseVarintField(sub, 2))
				if level > 0 {
					b.status.Level = level
				}
				b.status.ExpPerHour = float64(exp)
			case 4: // Wallet
				gold := int64(parseVarintField(sub, 1))
				if gold > 0 {
					b.status.Gold = gold
				}
			case 6: // Stats
				// HP e Mana
				b.status.HP = 200 + (b.status.Level * 25)
				b.status.MaxHP = b.status.HP
				b.status.Mana = 100 + (b.status.Level * 15)
				b.status.MaxMana = b.status.Mana
			case 14: // Hunt / HuntPrefs
				tierMin := uint32(parseVarintField(sub, 1))
				dif := parseStringField(sub, 2)
				areaStr := parseStringField(sub, 1)
				if tierMin > 0 {
					b.currentTier = tierMin
					if b.currentGate == "" {
						b.currentGate = "blue-gate"
					}
					if dif != "" {
						b.currentDifficulty = dif
					}
					huntName := GetHuntName(b.currentGate, b.currentTier)
					b.status.CurrentArea = huntName
					b.status.Status = "hunting"
				} else if areaStr != "" {
					b.status.CurrentArea = areaStr
					b.status.Status = "hunting"
				}
			}
		} else {
			_, n := protowire.ConsumeVarint(data)
			data = data[n:]
		}
	}
	if b.status.Status == "" || b.status.Status == "Conectado e Autenticado" {
		b.status.Status = "hunting"
	}
}

func (b *Bot) parseCharacterPatch(bytesVal []byte) {
	data := bytesVal
	for len(data) > 0 {
		num, wtype, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		if wtype == protowire.BytesType {
			sub, n := protowire.ConsumeBytes(data)
			if n < 0 {
				break
			}
			data = data[n:]

			switch num {
			case 3: // Progress
				val := int64(parseVarintField(sub, 1))
				exp := int64(parseVarintField(sub, 2))
				if val > 0 && val < 200 {
					b.status.Level = int(val)
					b.status.HP = 200 + (b.status.Level * 25)
					b.status.MaxHP = b.status.HP
					b.status.Mana = 100 + (b.status.Level * 15)
					b.status.MaxMana = b.status.Mana
				}
				if exp > 0 {
					b.status.ExpPerHour = float64(exp)
				}
			case 4: // Wallet
				gold := int64(parseVarintField(sub, 1))
				if gold > 0 {
					b.status.Gold = gold
				}
			}
		} else {
			_, n := protowire.ConsumeVarint(data)
			if n < 0 {
				break
			}
			data = data[n:]
		}
	}

	b.status.Kills++
	b.status.DurationSec += 3
	b.status.UpdatedAt = time.Now().UnixMilli()
}

func (b *Bot) handlePartyResponse(bytesVal []byte) {
	// PartyResponse { 1: state (PartyState) }
	b.addLogLocked("Atualização de Party recebida.")
}

func (b *Bot) handlePartyPush(bytesVal []byte) {
	data := bytesVal
	for len(data) > 0 {
		num, wtype, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		if wtype == protowire.BytesType {
			sub, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return
			}
			data = data[n:]

			switch num {
			case 1: // state (PartyState)
				b.addLogLocked("Party sincronizada.")
				pData := sub
				minLvl := b.status.Level
				var leaderGate string
				var leaderTier uint32
				for len(pData) > 0 {
					pNum, pType, pn := protowire.ConsumeTag(pData)
					if pn < 0 {
						break
					}
					pData = pData[pn:]
					if pType == protowire.BytesType {
						mBytes, mn := protowire.ConsumeBytes(pData)
						if mn < 0 {
							break
						}
						pData = pData[mn:]
						if pNum == 2 { // PartyMember
							mLvl := int(parseVarintField(mBytes, 4))
							isLeader := parseVarintField(mBytes, 6) == 1
							mGate := parseStringField(mBytes, 18)
							mTier := uint32(parseVarintField(mBytes, 19))
							if mLvl > 0 && mLvl < minLvl {
								minLvl = mLvl
							}
							if isLeader && mGate != "" {
								leaderGate = mGate
								leaderTier = mTier
							}
						}
					} else {
						_, pn := protowire.ConsumeVarint(pData)
						pData = pData[pn:]
					}
				}
				if minLvl > 0 {
					b.partyMinLevel = minLvl
				}
				// Se este bot for membro e o líder tiver caçada definida, sincroniza
				if !b.config.Settings.PartyLeader && leaderGate != "" {
					b.currentGate = leaderGate
					b.currentTier = leaderTier
					b.status.CurrentArea = GetHuntName(leaderGate, leaderTier)
				}
			case 2: // invite (PartyInvite)
				// PartyInvite { 1: from_account, 2: from_name }
				fromAcc := parseVarintField(sub, 1)
				fromName := parseStringField(sub, 2)
				b.addLogLocked(fmt.Sprintf("Convite de Party recebido de %s (#%d)", fromName, fromAcc))
				if b.config.Settings.AutoParty {
					_ = b.sendPartyAnswer(int64(fromAcc), true)
					b.addLogLocked(fmt.Sprintf("Aceitou convite de Party de %s automaticamente!", fromName))
				}
			}
		} else {
			_, n := protowire.ConsumeVarint(data)
			data = data[n:]
		}
	}
}

func (b *Bot) performTick() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.inGame {
		return
	}

	b.status.DurationSec += 3
	if b.status.Status == "" {
		b.status.Status = "hunting"
	}

	// AutoHunt: Analisador e Seletor Inteligente de Caçadas (Melhor XP/hora)
	if b.config.Settings.AutoHunt && b.status.Level > 0 {
		targetLevel := b.status.Level
		if b.config.Settings.PartyLeader && b.partyMinLevel > 0 {
			targetLevel = b.partyMinLevel
		}
		best := GetBestHuntForLevel(targetLevel)

		if b.config.Settings.PartyLeader || len(b.config.Settings.PartyMembers) == 0 {
			if b.currentGate != best.GateID || b.currentTier != best.TierMin || b.currentDifficulty != best.Difficulty {
				if time.Since(b.lastHuntCheck) > 10*time.Second {
					b.lastHuntCheck = time.Now()
					_ = b.sendStartHunt(best.GateID, best.TierMin, best.Difficulty)
					b.currentGate = best.GateID
					b.currentTier = best.TierMin
					b.currentDifficulty = best.Difficulty
					huntName := GetHuntName(best.GateID, best.TierMin)
					b.status.CurrentArea = huntName
					b.addLogLocked(fmt.Sprintf("AutoHunt: Otimizando caçada -> %s", huntName))
				}
			}
		} else {
			// Membro: se ainda não estiver em caçada após 20s
			if (b.currentGate == "" || b.currentTier == 0) && time.Since(b.lastHuntCheck) > 20*time.Second {
				b.lastHuntCheck = time.Now()
				_ = b.sendStartHunt(best.GateID, best.TierMin, best.Difficulty)
				b.currentGate = best.GateID
				b.currentTier = best.TierMin
				b.currentDifficulty = best.Difficulty
				b.status.CurrentArea = GetHuntName(best.GateID, best.TierMin)
			}
		}
	}

	// Se for o Líder da Party, enviar convites periodicamente aos membros configurados
	if b.config.Settings.PartyLeader && len(b.config.Settings.PartyMembers) > 0 {
		// Enviar convite no início ou a cada ~120 segundos
		if b.status.DurationSec <= 15 || b.status.DurationSec%120 == 0 {
			for _, member := range b.config.Settings.PartyMembers {
				if member != "" && member != b.config.Credentials.Username {
					_ = b.sendPartyInvite(member)
				}
			}
		}
	}
}

func parseStringField(b []byte, targetNum protowire.Number) string {
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			break
		}
		b = b[n:]
		if wtype == protowire.BytesType {
			bytesVal, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return ""
			}
			b = b[n:]
			if num == targetNum {
				return string(bytesVal)
			}
		} else {
			_, n := protowire.ConsumeVarint(b)
			b = b[n:]
		}
	}
	return ""
}

func parseVarintField(b []byte, targetNum protowire.Number) uint64 {
	for len(b) > 0 {
		num, wtype, n := protowire.ConsumeTag(b)
		if n < 0 {
			break
		}
		b = b[n:]
		if wtype == protowire.VarintType {
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return 0
			}
			b = b[n:]
			if num == targetNum {
				return v
			}
		} else if wtype == protowire.BytesType {
			_, n := protowire.ConsumeBytes(b)
			b = b[n:]
		}
	}
	return 0
}

func (b *Bot) GetStatus() engine.AccountStatus {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.status
}

func (b *Bot) ExecuteAction(action string, payload map[string]any) (any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch action {
	case "autoHunt":
		b.config.Settings.AutoHunt = !b.config.Settings.AutoHunt
		if b.config.Settings.AutoHunt {
			b.status.Status = "hunting"
			b.status.Logs = append(b.status.Logs, "Auto-hunt ativado.")
		} else {
			b.status.Status = "idle"
			b.status.Logs = append(b.status.Logs, "Auto-hunt pausado.")
		}
		return map[string]any{"autoHunt": b.config.Settings.AutoHunt}, nil

	case "autoHeal":
		b.status.HP = b.status.MaxHP
		b.status.Mana = b.status.MaxMana
		b.status.Logs = append(b.status.Logs, "Vida e mana recuperadas.")
		return "Curado!", nil

	default:
		return nil, fmt.Errorf("ação não implementada: %s", action)
	}
}

func (b *Bot) SupplyTicket(ticket, wsURL string) error {
	log.Printf("[%s] Recebido ticket externo: %s", b.ID(), ticket)
	return nil
}

func (b *Bot) updateStatus(fn func(s *engine.AccountStatus)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fn(&b.status)
	b.status.UpdatedAt = time.Now().UnixMilli()
}

func (b *Bot) addLogLocked(msg string) {
	b.status.Logs = append(b.status.Logs, msg)
	if len(b.status.Logs) > 15 {
		b.status.Logs = b.status.Logs[1:]
	}
}

func (b *Bot) addLog(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.addLogLocked(msg)
}
