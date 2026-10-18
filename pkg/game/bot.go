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

	// Controle de Caçadas (Auto-Hunt) & Bestiário (Codex)
	currentGate          string
	currentTier          uint32
	currentDifficulty    string
	partyMinLevel        int
	lastHuntCheck        time.Time
	lastAttemptedHuntKey string
	codexKills           map[string]uint64
	activePartyMembers   map[string]bool
	huntConfirmed        bool

	// Controle de Progressão Completa (Zero Limitações)
	statPoints           uint32
	talentPoints         uint32
	automationConfigured bool
	lastEquipCheck       time.Time
	lastRepairCheck      time.Time
	lastMissionCheck     time.Time
	lastShadowCheck      time.Time
	lastPromotionCheck   time.Time

	// Métricas de EXP real e cálculo de ExpPerHour
	xpStartTime   time.Time
	totalXPGained uint64
	lastExp       int64
	lastLevel     int
}

type PartyCoordinator struct {
	mu           sync.RWMutex
	LeaderGate   string
	LeaderTier   uint32
	LeaderDif    string
	Levels       map[string]int
	MonsterKills map[string]map[string]uint64
	LockedHunts  map[string]time.Time
}

var GlobalParty = &PartyCoordinator{
	Levels:       make(map[string]int),
	MonsterKills: make(map[string]map[string]uint64),
	LockedHunts:  make(map[string]time.Time),
}

func (p *PartyCoordinator) MarkHuntLocked(key string, duration time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.LockedHunts == nil {
		p.LockedHunts = make(map[string]time.Time)
	}
	p.LockedHunts[key] = time.Now().Add(duration)
}

func (p *PartyCoordinator) GetLockedHunts() map[string]time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cp := make(map[string]time.Time)
	now := time.Now()
	for k, v := range p.LockedHunts {
		if v.After(now) {
			cp[k] = v
		}
	}
	return cp
}

func (p *PartyCoordinator) ReportLevel(botID string, level int) {
	if level <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Levels[botID] = level
}

func (p *PartyCoordinator) ReportKills(botID string, kills map[string]uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.MonsterKills[botID] == nil {
		p.MonsterKills[botID] = make(map[string]uint64)
	}
	for k, v := range kills {
		p.MonsterKills[botID][k] = v
	}
}

func (p *PartyCoordinator) GetMinLevel(fallback int) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.Levels) == 0 {
		return fallback
	}
	minLvl := 999
	for _, lvl := range p.Levels {
		if lvl > 0 && lvl < minLvl {
			minLvl = lvl
		}
	}
	if minLvl == 999 {
		return fallback
	}
	return minLvl
}

func (p *PartyCoordinator) GetSharedCodexKills() map[string]uint64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	minKills := make(map[string]uint64)
	for _, tier := range CatalogTiers {
		var lowest uint64 = 99999999
		foundAny := false
		for _, botMap := range p.MonsterKills {
			foundAny = true
			k := botMap[tier.MonsterID]
			if k < lowest {
				lowest = k
			}
		}
		if !foundAny || lowest == 99999999 {
			lowest = 0
		}
		minKills[tier.MonsterID] = lowest
	}
	return minKills
}

func (p *PartyCoordinator) SetLeaderHunt(gate string, tier uint32, dif string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LeaderGate = gate
	p.LeaderTier = tier
	p.LeaderDif = dif
}

func (p *PartyCoordinator) GetLeaderHunt() (string, uint32, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.LeaderGate, p.LeaderTier, p.LeaderDif
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
		codexKills: make(map[string]uint64),
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

// sendPartyLeave envia requisição para sair da Party atual
func (b *Bot) sendPartyLeave() error {
	reqID := b.nextReqID()
	// PartyRequest { 4: leave (PartyLeave) }
	var partyReq []byte
	partyReq = protowire.AppendTag(partyReq, 4, protowire.BytesType)
	partyReq = protowire.AppendBytes(partyReq, []byte{})

	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 290, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, partyReq)

	log.Printf("[%s] Saindo da Party atual", b.ID())
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

	b.lastAttemptedHuntKey = fmt.Sprintf("%s:%d:%s", gateID, tierMin, difficulty)
	log.Printf("[%s] Enviando StartHunt (gate=%s, tier=%d, dif=%s)", b.ID(), gateID, tierMin, difficulty)
	return b.socket.SendBinary(clientMsg)
}

// sendGiveUp envia desistência quando o personagem morre em combate (Tag 36)
func (b *Bot) sendGiveUp() error {
	reqID := b.nextReqID()
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 36, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, []byte{})

	log.Printf("[%s] Enviando GiveUp (reiniciar combate após derrota)", b.ID())
	return b.socket.SendBinary(clientMsg)
}

// sendRevive tenta reviver o personagem se houver consumível (Tag 35)
func (b *Bot) sendRevive() error {
	reqID := b.nextReqID()
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 35, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, []byte{})

	log.Printf("[%s] Enviando Revive", b.ID())
	return b.socket.SendBinary(clientMsg)
}

// sendEquipBest equipa automaticamente os melhores itens do inventário (Tag 54)
func (b *Bot) sendEquipBest() error {
	reqID := b.nextReqID()

	// EquipBestApply { 1: criterion = 2 (EQUIP_CRITERION_POWER) }
	var equipApply []byte
	equipApply = protowire.AppendTag(equipApply, 1, protowire.VarintType)
	equipApply = protowire.AppendVarint(equipApply, 2)

	// ClientMessage { 1: req_id, 54: equip_best_apply }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 54, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, equipApply)

	log.Printf("[%s] Enviando EquipBest (auto-equipar melhores itens)", b.ID())
	return b.socket.SendBinary(clientMsg)
}

// sendAllocateStats distribui pontos de atributo conforme a vocação (Tag 90)
// StatId enum: STR=1, AGI=2, VIT=3, INT=6, PER=7
func (b *Bot) sendAllocateStats(points uint32) error {
	if points == 0 {
		return nil
	}
	reqID := b.nextReqID()

	voc := b.status.Vocation
	if voc == "" {
		voc = b.config.Settings.PreferredClass
	}

	var statAmounts [][]byte

	switch voc {
	case "mago":
		// 80% INT (6), 20% VIT (3)
		intPts := uint32(float64(points) * 0.8)
		vitPts := points - intPts
		if intPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(6, intPts))
		}
		if vitPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(3, vitPts))
		}

	case "healer", "sacerdote":
		// 70% INT (6), 30% VIT (3)
		intPts := uint32(float64(points) * 0.7)
		vitPts := points - intPts
		if intPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(6, intPts))
		}
		if vitPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(3, vitPts))
		}

	case "arqueiro":
		// 70% AGI (2), 30% PER (7)
		agiPts := uint32(float64(points) * 0.7)
		perPts := points - agiPts
		if agiPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(2, agiPts))
		}
		if perPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(7, perPts))
		}

	case "assassino":
		// 60% AGI (2), 40% PER (7)
		agiPts := uint32(float64(points) * 0.6)
		perPts := points - agiPts
		if agiPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(2, agiPts))
		}
		if perPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(7, perPts))
		}

	default: // guerreiro ou sem vocação definida -> STR + VIT
		// 80% STR (1), 20% VIT (3)
		strPts := uint32(float64(points) * 0.8)
		vitPts := points - strPts
		if strPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(1, strPts))
		}
		if vitPts > 0 {
			statAmounts = append(statAmounts, buildStatAmount(3, vitPts))
		}
	}

	// AllocateStats { 1: repeated StatAmount points }
	var allocMsg []byte
	for _, sa := range statAmounts {
		allocMsg = protowire.AppendTag(allocMsg, 1, protowire.BytesType)
		allocMsg = protowire.AppendBytes(allocMsg, sa)
	}

	// ClientMessage { 1: req_id, 90: allocate_stats }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 90, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, allocMsg)

	log.Printf("[%s] Distribuindo %d pontos de atributo para vocação '%s'", b.ID(), points, voc)
	b.addLogLocked(fmt.Sprintf("Atributos: Distribuídos %d pontos (%s)", points, voc))
	return b.socket.SendBinary(clientMsg)
}

func buildStatAmount(statID uint32, points uint32) []byte {
	var sa []byte
	sa = protowire.AppendTag(sa, 1, protowire.VarintType)
	sa = protowire.AppendVarint(sa, uint64(statID))
	sa = protowire.AppendTag(sa, 2, protowire.VarintType)
	sa = protowire.AppendVarint(sa, uint64(points))
	return sa
}

// sendRepairAll repara todos os equipamentos do personagem (Tag 65)
func (b *Bot) sendRepairAll() error {
	reqID := b.nextReqID()

	// Repair { 1: repeated uids, 2: partial = false }
	// Deixar uids vazio com partial=false repara todo o inventário/set equipado
	var repairMsg []byte
	repairMsg = protowire.AppendTag(repairMsg, 2, protowire.VarintType)
	repairMsg = protowire.AppendVarint(repairMsg, 0)

	// ClientMessage { 1: req_id, 65: repair }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 65, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, repairMsg)

	log.Printf("[%s] Enviando Repair (reparar todos os itens)", b.ID())
	return b.socket.SendBinary(clientMsg)
}

// sendClaimMissions resgata recompensas de missões (Journey e Daily) (Tag 410)
func (b *Bot) sendClaimMissions() error {
	reqID := b.nextReqID()

	// MissionsRequest { 1: state = true, 2: claim_journey = true, 3: claim_daily = "" }
	var missionsReq []byte
	missionsReq = protowire.AppendTag(missionsReq, 1, protowire.VarintType)
	missionsReq = protowire.AppendVarint(missionsReq, 1)
	missionsReq = protowire.AppendTag(missionsReq, 2, protowire.VarintType)
	missionsReq = protowire.AppendVarint(missionsReq, 1)
	missionsReq = protowire.AppendTag(missionsReq, 3, protowire.BytesType)
	missionsReq = protowire.AppendString(missionsReq, "daily")

	// ClientMessage { 1: req_id, 410: missions_request }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 410, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, missionsReq)

	log.Printf("[%s] Enviando ClaimMissions (resgate de missões diárias e jornada)", b.ID())
	return b.socket.SendBinary(clientMsg)
}

// sendSetAutomation configura a automação completa no servidor (Tag 98)
// ativa auto_continue, auto_arise, auto_chest, poções e auto_sell
func (b *Bot) sendSetAutomation() error {
	reqID := b.nextReqID()

	// AutoPotion for HP (threshold 0.5)
	var hpPotion []byte
	hpPotion = protowire.AppendTag(hpPotion, 2, protowire.Fixed32Type)
	hpPotion = protowire.AppendFixed32(hpPotion, 0x3f000000) // float32(0.5)
	hpPotion = protowire.AppendTag(hpPotion, 4, protowire.VarintType)
	hpPotion = protowire.AppendVarint(hpPotion, 1) // best_fit = true

	// AutoPotion for Mana (threshold 0.3)
	var manaPotion []byte
	manaPotion = protowire.AppendTag(manaPotion, 2, protowire.Fixed32Type)
	manaPotion = protowire.AppendFixed32(manaPotion, 0x3e99999a) // float32(0.3)
	manaPotion = protowire.AppendTag(manaPotion, 4, protowire.VarintType)
	manaPotion = protowire.AppendVarint(manaPotion, 1) // best_fit = true

	// Automation { 2: auto_continue, 5: auto_arise, 6: auto_chest, 7: hp_potion, 8: mana_potion, 9: auto_shadow_potion, 10: auto_sell, 16: auto_shadow_revive }
	var autoMsg []byte
	autoMsg = protowire.AppendTag(autoMsg, 2, protowire.VarintType)
	autoMsg = protowire.AppendVarint(autoMsg, 1) // auto_continue
	if b.status.Level >= 15 {
		autoMsg = protowire.AppendTag(autoMsg, 5, protowire.VarintType)
		autoMsg = protowire.AppendVarint(autoMsg, 1) // auto_arise (ressuscitar sombras)
	}
	autoMsg = protowire.AppendTag(autoMsg, 6, protowire.VarintType)
	autoMsg = protowire.AppendVarint(autoMsg, 1) // auto_chest (abrir baús)
	autoMsg = protowire.AppendTag(autoMsg, 7, protowire.BytesType)
	autoMsg = protowire.AppendBytes(autoMsg, hpPotion)
	autoMsg = protowire.AppendTag(autoMsg, 8, protowire.BytesType)
	autoMsg = protowire.AppendBytes(autoMsg, manaPotion)
	if b.status.Level >= 15 {
		autoMsg = protowire.AppendTag(autoMsg, 9, protowire.VarintType)
		autoMsg = protowire.AppendVarint(autoMsg, 1) // auto_shadow_potion
	}
	autoMsg = protowire.AppendTag(autoMsg, 10, protowire.VarintType)
	autoMsg = protowire.AppendVarint(autoMsg, 1) // auto_sell
	if b.status.Level >= 15 {
		autoMsg = protowire.AppendTag(autoMsg, 16, protowire.VarintType)
		autoMsg = protowire.AppendVarint(autoMsg, 1) // auto_shadow_revive
	}

	// SetAutomation { 1: automation }
	var setAuto []byte
	setAuto = protowire.AppendTag(setAuto, 1, protowire.BytesType)
	setAuto = protowire.AppendBytes(setAuto, autoMsg)

	// ClientMessage { 1: req_id, 98: set_automation }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 98, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, setAuto)

	log.Printf("[%s] Enviando SetAutomation (auto_arise, auto_chest, auto_sell, potions)", b.ID())
	b.addLogLocked("Servidor: Automação completa ativada (Arise, Baús, Poções, Auto-Sell)")
	return b.socket.SendBinary(clientMsg)
}

// sendShadowTeamBest escala automaticamente o melhor time de sombras (Tag 121)
func (b *Bot) sendShadowTeamBest() error {
	reqID := b.nextReqID()

	// ShadowSetTeam { 1: action = 5 (SHADOW_TEAM_ACTION_BEST) }
	var shadowTeam []byte
	shadowTeam = protowire.AppendTag(shadowTeam, 1, protowire.VarintType)
	shadowTeam = protowire.AppendVarint(shadowTeam, 5)

	// ClientMessage { 1: req_id, 121: shadow_set_team }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 121, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, shadowTeam)

	log.Printf("[%s] Enviando ShadowTeamBest (escalar melhores sombras)", b.ID())
	return b.socket.SendBinary(clientMsg)
}

// sendShadowEquipBest equipa automaticamente os melhores itens nas sombras (Tag 127)
func (b *Bot) sendShadowEquipBest() error {
	reqID := b.nextReqID()

	// ShadowEquipBest {}
	// ClientMessage { 1: req_id, 127: shadow_equip_best }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 127, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, []byte{})

	log.Printf("[%s] Enviando ShadowEquipBest (equipar melhores itens nas sombras)", b.ID())
	return b.socket.SendBinary(clientMsg)
}

// sendChooseClass escolhe a classe do caçador caso ainda seja aprendiz (Tag 93)
func (b *Bot) sendChooseClass(classID string) error {
	if classID == "" {
		return nil
	}
	reqID := b.nextReqID()

	// ChooseClass { 1: class_id }
	var chooseClass []byte
	chooseClass = protowire.AppendTag(chooseClass, 1, protowire.BytesType)
	chooseClass = protowire.AppendString(chooseClass, classID)

	// ClientMessage { 1: req_id, 93: choose_class }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 93, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, chooseClass)

	log.Printf("[%s] Escolhendo classe: %s", b.ID(), classID)
	b.addLogLocked(fmt.Sprintf("Classe escolhida: %s", classID))
	return b.socket.SendBinary(clientMsg)
}

// sendStartPromotion inicia a batalha de promoção de Rank (Tag 99)
func (b *Bot) sendStartPromotion() error {
	reqID := b.nextReqID()

	// Promotion { 2: start = true }
	var promoMsg []byte
	promoMsg = protowire.AppendTag(promoMsg, 2, protowire.VarintType)
	promoMsg = protowire.AppendVarint(promoMsg, 1)

	// ClientMessage { 1: req_id, 99: promotion }
	var clientMsg []byte
	clientMsg = protowire.AppendTag(clientMsg, 1, protowire.VarintType)
	clientMsg = protowire.AppendVarint(clientMsg, uint64(reqID))
	clientMsg = protowire.AppendTag(clientMsg, 99, protowire.BytesType)
	clientMsg = protowire.AppendBytes(clientMsg, promoMsg)

	log.Printf("[%s] Enviando StartPromotion (tentar subida de Rank)", b.ID())
	b.addLogLocked("Promoção de Rank solicitada!")
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
				} else if errMsg == "shadow.notFound" || errMsg == "missions.unknown" || errMsg == "request.invalid" || errMsg == "repair.nothing" {
					// Avisos normais quando ainda não possui sombras, missões ou nada para reparar
					b.addLogLocked(fmt.Sprintf("Info servidor: %s", errMsg))
				} else {
					b.status.Status = fmt.Sprintf("Erro: %s", errMsg)
					b.addLogLocked(fmt.Sprintf("Servidor erro: %s", errMsg))
					if b.lastAttemptedHuntKey != "" && (errMsg == "level.tooLow" || errMsg == "gate.locked" || errMsg == "hunt.locked" || errMsg == "tier.invalid") {
						GlobalParty.MarkHuntLocked(b.lastAttemptedHuntKey, 15*time.Minute)
						b.addLogLocked(fmt.Sprintf("Codex: Caçada %s bloqueada pelo servidor (%s). Avançando para o próximo monstro!", b.lastAttemptedHuntKey, errMsg))
						b.lastAttemptedHuntKey = ""
					}
				}

			case 4: // AuthOk
				token := parseStringField(bytesVal, 1)
				username := parseStringField(bytesVal, 3)
				b.token = token
				b.inGame = true
				b.status.Status = "Conectado e Autenticado"
				b.status.Name = username
				b.addLogLocked(fmt.Sprintf("Login bem-sucedido! Bem-vindo, %s.", username))

			case 6: // SessionEnded
				b.huntConfirmed = false
				b.currentGate = ""
				b.currentTier = 0
				b.addLogLocked("Sessão finalizada pelo servidor.")

			case 8: // CharacterState
				b.parseCharacterState(bytesVal)

			case 9: // CharacterPatch
				b.parseCharacterPatch(bytesVal)

			case 14: // WorldEvents
				b.status.Kills++
				b.status.Gold += 25
				b.status.UpdatedAt = time.Now().UnixMilli()

			case 30: // StartHuntResult
				b.lastAttemptedHuntKey = ""
				b.huntConfirmed = true
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

			case 31: // LeaveHuntResult
				b.huntConfirmed = false
				b.currentGate = ""
				b.currentTier = 0
				b.addLogLocked("Saiu da caçada.")

			case 36: // GiveUpResult
				b.huntConfirmed = false
				b.currentGate = ""
				b.currentTier = 0
				b.addLogLocked("Desistência confirmada. Retomando caçada...")

			case 41: // HuntSession
				b.lastAttemptedHuntKey = ""
				// Metadados informativos de caçada anterior. NÃO confirma caçada na sessão ativa!
				gateID := parseStringField(bytesVal, 1)
				tierMin := uint32(parseVarintField(bytesVal, 2))
				if gateID != "" {
					b.status.CurrentArea = GetHuntName(gateID, tierMin)
				}

			case 43: // DeathState
				b.huntConfirmed = false
				b.currentGate = ""
				b.currentTier = 0
				b.addLogLocked("Personagem derrotado em combate! Reiniciando caçada...")
				_ = b.sendGiveUp()

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
				b.lastLevel = level
				b.lastExp = exp
				if b.xpStartTime.IsZero() {
					b.xpStartTime = time.Now()
				}
			case 4: // Wallet
				gold := int64(parseVarintField(sub, 1))
				if gold > 0 {
					b.status.Gold = gold
				}
			case 5: // Points
				b.statPoints = uint32(parseVarintField(sub, 1))
				b.talentPoints = uint32(parseVarintField(sub, 2))
			case 6: // Stats
				// HP e Mana
				b.status.HP = 200 + (b.status.Level * 25)
				b.status.MaxHP = b.status.HP
				b.status.Mana = 100 + (b.status.Level * 15)
				b.status.MaxMana = b.status.Mana
			case 14: // Hunt / HuntPrefs
				tierMin := uint32(parseVarintField(sub, 1))
				areaStr := parseStringField(sub, 1)
				if tierMin > 0 {
					huntName := GetHuntName("blue-gate", tierMin)
					b.status.CurrentArea = huntName
				} else if areaStr != "" {
					b.status.CurrentArea = areaStr
				}
			case 19: // Codex (CodexState)
				kills := parseCodexMonsters(sub)
				for k, v := range kills {
					if v > b.codexKills[k] {
						b.codexKills[k] = v
					}
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
	if b.status.Level > 0 {
		GlobalParty.ReportLevel(b.ID(), b.status.Level)
	}
	GlobalParty.ReportKills(b.ID(), b.codexKills)
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
			case 2: // Progress
				val := int(parseVarintField(sub, 1))
				exp := int64(parseVarintField(sub, 2))
				if val > 0 && val < 200 {
					b.status.Level = val
					b.status.HP = 200 + (b.status.Level * 25)
					b.status.MaxHP = b.status.HP
					b.status.Mana = 100 + (b.status.Level * 15)
					b.status.MaxMana = b.status.Mana
				}
				if b.xpStartTime.IsZero() {
					b.xpStartTime = time.Now()
					b.lastLevel = val
					b.lastExp = exp
				} else {
					var gain int64
					if val > b.lastLevel {
						gain = exp + 500
						b.addLogLocked(fmt.Sprintf("🎉 LEVEL UP! Nível %d alcançado!", val))
					} else if exp > b.lastExp {
						gain = exp - b.lastExp
					}
					if gain > 0 {
						b.totalXPGained += uint64(gain)
						elapsed := time.Since(b.xpStartTime).Hours()
						if elapsed > 0.001 {
							b.status.ExpPerHour = float64(b.totalXPGained) / elapsed
						}
					}
					b.lastLevel = val
					b.lastExp = exp
				}
			case 3: // Wallet
				gold := int64(parseVarintField(sub, 1))
				if gold > 0 {
					b.status.Gold = gold
				}
			case 4: // Points
				b.statPoints = uint32(parseVarintField(sub, 1))
				b.talentPoints = uint32(parseVarintField(sub, 2))
			case 16: // Codex (CodexState)
				kills := parseCodexMonsters(sub)
				for k, v := range kills {
					if v > b.codexKills[k] {
						b.codexKills[k] = v
					}
				}
			case 17: // CodexKills
				kills := parseCodexMonsters(sub)
				for k, v := range kills {
					if v > b.codexKills[k] {
						b.codexKills[k] = v
					}
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

	if b.currentGate != "" && b.currentTier > 0 {
		for _, tier := range CatalogTiers {
			if tier.GateID == b.currentGate && tier.TierMin == b.currentTier {
				b.codexKills[tier.MonsterID]++
				break
			}
		}
	}

	if b.status.Level > 0 {
		GlobalParty.ReportLevel(b.ID(), b.status.Level)
	}
	GlobalParty.ReportKills(b.ID(), b.codexKills)
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
				activeMembers := make(map[string]bool)
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
							mName := parseStringField(mBytes, 2)
							mLvl := int(parseVarintField(mBytes, 4))
							isLeader := parseVarintField(mBytes, 6) == 1
							mGate := parseStringField(mBytes, 18)
							mTier := uint32(parseVarintField(mBytes, 19))
							if mName != "" {
								activeMembers[mName] = true
							}
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
				b.activePartyMembers = activeMembers
				if minLvl > 0 {
					b.partyMinLevel = minLvl
				}
				// Se este bot for membro e o líder tiver caçada definida, sincroniza
				if !b.config.Settings.PartyLeader && leaderGate != "" && leaderTier > 0 {
					if !b.huntConfirmed || b.currentGate != leaderGate || b.currentTier != leaderTier {
						_ = b.sendStartHunt(leaderGate, leaderTier, "normal")
						b.status.CurrentArea = GetHuntName(leaderGate, leaderTier)
						b.addLogLocked(fmt.Sprintf("Auto-Follow: Sincronizando caçada com o líder -> %s", b.status.CurrentArea))
					}
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

	if b.status.Level > 0 {
		GlobalParty.ReportLevel(b.ID(), b.status.Level)
	}
	GlobalParty.ReportKills(b.ID(), b.codexKills)

	// AutoHunt: Analisador e Seletor Inteligente de Caçadas (Melhor XP/hora OU Codex/Completista)
	if b.config.Settings.AutoHunt && b.status.Level > 0 {
		if b.config.Settings.PartyLeader || len(b.config.Settings.PartyMembers) == 0 {
			targetLevel := GlobalParty.GetMinLevel(b.status.Level)

			var best HuntTier
			if b.config.Settings.CodexMode {
				targetMilestone := b.config.Settings.CodexTarget
				if targetMilestone == 0 {
					targetMilestone = 2000
				}
				sharedKills := GlobalParty.GetSharedCodexKills()
				lockedHunts := GlobalParty.GetLockedHunts()
				tier, pending, completed, total := GetNextCodexHuntForLevel(targetLevel, sharedKills, targetMilestone, lockedHunts)
				best = tier
				if pending {
					b.status.CodexInfo = fmt.Sprintf("%d/%d mobs | %s (%d/%d)", completed, total, tier.MonsterName, sharedKills[tier.MonsterID], targetMilestone)
				} else {
					b.status.CodexInfo = fmt.Sprintf("Completo (%d/%d mobs) | Foco XP", completed, total)
				}
			} else {
				best = GetBestHuntForLevel(targetLevel)
				b.status.CodexInfo = "Desativado"
			}

			// Publica caçada do líder para todos os membros no processo
			GlobalParty.SetLeaderHunt(best.GateID, best.TierMin, best.Difficulty)

			if !b.huntConfirmed || b.currentGate != best.GateID || b.currentTier != best.TierMin || b.currentDifficulty != best.Difficulty {
				if time.Since(b.lastHuntCheck) > 5*time.Second {
					b.lastHuntCheck = time.Now()
					_ = b.sendStartHunt(best.GateID, best.TierMin, best.Difficulty)
					huntName := GetHuntName(best.GateID, best.TierMin)
					b.status.CurrentArea = huntName
					if b.config.Settings.CodexMode {
						b.addLogLocked(fmt.Sprintf("Codex Líder: Focando em %s (Nv Min: %d)", huntName, targetLevel))
					} else {
						b.addLogLocked(fmt.Sprintf("AutoHunt Líder: Otimizando caçada -> %s", huntName))
					}
				}
			}
		} else {
			// Membro da party: SEGUIR O LÍDER RIGOROSAMENTE!
			targetGate, targetTier, targetDif := GlobalParty.GetLeaderHunt()

			if targetGate != "" && targetTier > 0 {
				if !b.huntConfirmed || b.currentGate != targetGate || b.currentTier != targetTier || b.currentDifficulty != targetDif {
					if time.Since(b.lastHuntCheck) > 4*time.Second {
						b.lastHuntCheck = time.Now()
						_ = b.sendStartHunt(targetGate, targetTier, targetDif)
						huntName := GetHuntName(targetGate, targetTier)
						b.status.CurrentArea = huntName
						b.addLogLocked(fmt.Sprintf("Auto-Follow: Sincronizando com líder -> %s", huntName))
					}
				}
			}

			// Atualiza CodexInfo do membro com o status compartilhado da party
			if b.config.Settings.CodexMode {
				targetMilestone := b.config.Settings.CodexTarget
				if targetMilestone == 0 {
					targetMilestone = 2000
				}
				targetLevel := GlobalParty.GetMinLevel(b.status.Level)
				sharedKills := GlobalParty.GetSharedCodexKills()
				lockedHunts := GlobalParty.GetLockedHunts()
				tier, pending, completed, total := GetNextCodexHuntForLevel(targetLevel, sharedKills, targetMilestone, lockedHunts)
				if pending {
					b.status.CodexInfo = fmt.Sprintf("%d/%d mobs | %s (%d/%d)", completed, total, tier.MonsterName, b.codexKills[tier.MonsterID], targetMilestone)
				} else {
					b.status.CodexInfo = fmt.Sprintf("Completo (%d/%d mobs) | Foco XP", completed, total)
				}
			} else {
				b.status.CodexInfo = "Desativado"
			}
		}
	}

	// Se for o Líder da Party, manter a party completa convidando APENAS quem ainda não entrou
	if b.config.Settings.PartyLeader && len(b.config.Settings.PartyMembers) > 0 {
		// Envia convite a cada ~60 segundos apenas para membros ausentes
		if b.status.DurationSec <= 15 || b.status.DurationSec%60 == 0 {
			for _, member := range b.config.Settings.PartyMembers {
				if member != "" && member != b.config.Credentials.Username {
					if b.activePartyMembers == nil || !b.activePartyMembers[member] {
						_ = b.sendPartyInvite(member)
					}
				}
			}
		}
	}

	// ==========================================
	// AUTONOMOUS PROGRESSION (Zero Limitações)
	// ==========================================

	// 1. Configurar Automações do Servidor (Arise, Baús, Poções, Auto-Sell)
	if !b.automationConfigured {
		b.automationConfigured = true
		_ = b.sendSetAutomation()
	}

	// 2. Escolha Automática de Classe se ainda não tiver e tiver configurada
	if (b.status.Vocation == "" || b.status.Vocation == "aprendiz") && b.config.Settings.PreferredClass != "" {
		_ = b.sendChooseClass(b.config.Settings.PreferredClass)
	}

	// 3. Distribuição Automática de Atributos (AutoStats)
	// Por padrão ativo, distribui todos os statPoints acumulados
	if (b.config.Settings.AutoStats || true) && b.statPoints > 0 {
		pts := b.statPoints
		b.statPoints = 0
		_ = b.sendAllocateStats(pts)
	}

	// 4. Auto-Equip Melhores Itens a cada 30 segundos
	if (b.config.Settings.AutoEquip || true) && time.Since(b.lastEquipCheck) > 30*time.Second {
		b.lastEquipCheck = time.Now()
		_ = b.sendEquipBest()
	}

	// 5. Auto-Repair Equipamentos a cada 3 minutos
	if (b.config.Settings.AutoRepair || true) && time.Since(b.lastRepairCheck) > 3*time.Minute {
		b.lastRepairCheck = time.Now()
		_ = b.sendRepairAll()
	}

	// 6. Resgatar Missões (Journey / Daily) a cada 60 segundos
	if (b.config.Settings.AutoMissions || true) && time.Since(b.lastMissionCheck) > 60*time.Second {
		b.lastMissionCheck = time.Now()
		_ = b.sendClaimMissions()
	}

	// 7. Auto-Shadows: Escalar Melhores Sombras e Equipar a cada 60 segundos
	if (b.config.Settings.AutoShadows || true) && time.Since(b.lastShadowCheck) > 60*time.Second {
		b.lastShadowCheck = time.Now()
		_ = b.sendShadowTeamBest()
		_ = b.sendShadowEquipBest()
	}

	// 8. Auto-Promotion: Tentar Promoção de Rank a cada 5 minutos
	if b.config.Settings.AutoPromotion && time.Since(b.lastPromotionCheck) > 5*time.Minute {
		b.lastPromotionCheck = time.Now()
		_ = b.sendStartPromotion()
	}
}

func parseCodexMonsters(data []byte) map[string]uint64 {
	res := make(map[string]uint64)
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
			if num == 1 { // repeated MonstersEntry
				mEntry := sub
				var key string
				var val uint64
				for len(mEntry) > 0 {
					eNum, eWtype, en := protowire.ConsumeTag(mEntry)
					if en < 0 {
						break
					}
					mEntry = mEntry[en:]
					if eWtype == protowire.BytesType {
						eSub, en2 := protowire.ConsumeBytes(mEntry)
						if en2 < 0 {
							break
						}
						mEntry = mEntry[en2:]
						if eNum == 1 {
							key = string(eSub)
						}
					} else if eWtype == protowire.VarintType {
						v, en2 := protowire.ConsumeVarint(mEntry)
						if en2 < 0 {
							break
						}
						mEntry = mEntry[en2:]
						if eNum == 2 {
							val = v
						}
					} else {
						break
					}
				}
				if key != "" {
					res[key] = val
				}
			}
		} else if wtype == protowire.VarintType {
			_, n := protowire.ConsumeVarint(data)
			if n < 0 {
				break
			}
			data = data[n:]
		} else {
			break
		}
	}
	return res
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

	case "testHunt":
		gate, _ := payload["gate"].(string)
		tierFloat, _ := payload["tier"].(float64)
		dif, _ := payload["difficulty"].(string)
		if dif == "" {
			dif = "normal"
		}
		tierMin := uint32(tierFloat)
		err := b.sendStartHunt(gate, tierMin, dif)
		b.addLogLocked(fmt.Sprintf("Ação Manual testHunt enviada: Gate=%s Tier=%d Dif=%s", gate, tierMin, dif))
		return map[string]any{"sent": true, "gate": gate, "tier": tierMin, "difficulty": dif}, err

	case "equipBest":
		err := b.sendEquipBest()
		b.addLogLocked("Ação Manual: Equipar Melhores Itens executado.")
		return map[string]any{"executed": true}, err

	case "allocateStats":
		points := b.statPoints
		if pVal, ok := payload["points"].(float64); ok && pVal > 0 {
			points = uint32(pVal)
		}
		if points == 0 {
			points = 5 // mínimo para teste
		}
		err := b.sendAllocateStats(points)
		b.statPoints = 0
		return map[string]any{"executed": true, "points": points}, err

	case "repair":
		err := b.sendRepairAll()
		b.addLogLocked("Ação Manual: Reparar Todos os Equipamentos executado.")
		return map[string]any{"executed": true}, err

	case "claimMissions":
		err := b.sendClaimMissions()
		b.addLogLocked("Ação Manual: Resgatar Missões executado.")
		return map[string]any{"executed": true}, err

	case "setAutomation":
		err := b.sendSetAutomation()
		return map[string]any{"executed": true}, err

	case "shadowTeam":
		err := b.sendShadowTeamBest()
		_ = b.sendShadowEquipBest()
		b.addLogLocked("Ação Manual: Otimizar Sombras executado.")
		return map[string]any{"executed": true}, err

	case "promotion":
		err := b.sendStartPromotion()
		return map[string]any{"executed": true}, err

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
