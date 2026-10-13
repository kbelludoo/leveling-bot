package game

import "fmt"

// HuntTier representa uma faixa de caçada / portal no Leveling Idle 2D
type HuntTier struct {
	GateID        string
	GateName      string
	TierMin       uint32
	TierMax       uint32
	MonsterID     string
	MonsterName   string
	BossName      string
	Difficulty    string
	AreaLevel     uint32
	RequiredLevel uint32
}

// CatalogTiers lista completa extraída do ContentCatalog do jogo
var CatalogTiers = []HuntTier{
	// --- Normal (Níveis 1 a 70) ---
	// Blue Gate (Nível Mínimo 1)
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 1, TierMax: 5, MonsterID: "rato-ruina", MonsterName: "Rato da Ruína", BossName: "Rato Gigante", Difficulty: "normal", AreaLevel: 5, RequiredLevel: 1},
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 6, TierMax: 10, MonsterID: "morcego", MonsterName: "Morcego Sombrio", BossName: "Morcego Ancestral", Difficulty: "normal", AreaLevel: 10, RequiredLevel: 6},
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 11, TierMax: 15, MonsterID: "goblin", MonsterName: "Goblin", BossName: "Goblin Chefão", Difficulty: "normal", AreaLevel: 15, RequiredLevel: 11},
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 16, TierMax: 20, MonsterID: "esqueleto-guerreiro", MonsterName: "Esqueleto Guerreiro", BossName: "Esqueleto Campeão", Difficulty: "normal", AreaLevel: 20, RequiredLevel: 16},

	// Castelo dos Demônios (Nível Mínimo 10)
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 21, TierMax: 25, MonsterID: "orc-jovem", MonsterName: "Orc Jovem", BossName: "Orc Senhor da Guerra", Difficulty: "normal", AreaLevel: 25, RequiredLevel: 21},
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 26, TierMax: 30, MonsterID: "zumbi-faminto", MonsterName: "Zumbi Faminto", BossName: "Zumbi Devorador", Difficulty: "normal", AreaLevel: 30, RequiredLevel: 26},
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 31, TierMax: 35, MonsterID: "slime-corrosivo", MonsterName: "Slime Corrosivo", BossName: "Slime Colossal", Difficulty: "normal", AreaLevel: 35, RequiredLevel: 31},
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 36, TierMax: 40, MonsterID: "aranha-pedra", MonsterName: "Aranha de Pedra", BossName: "Aranha Matriarca", Difficulty: "normal", AreaLevel: 40, RequiredLevel: 36},

	// Ilha Jeju (Nível Mínimo 15)
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 41, TierMax: 45, MonsterID: "golem-barro", MonsterName: "Golem de Barro", BossName: "Golem Ancestral", Difficulty: "normal", AreaLevel: 45, RequiredLevel: 41},
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 46, TierMax: 50, MonsterID: "espirito-errante", MonsterName: "Espírito Errante", BossName: "Espírito Carrasco", Difficulty: "normal", AreaLevel: 50, RequiredLevel: 46},
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 51, TierMax: 55, MonsterID: "cavaleiro-morto", MonsterName: "Cavaleiro Morto", BossName: "Comandante Caído", Difficulty: "normal", AreaLevel: 55, RequiredLevel: 51},
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 56, TierMax: 60, MonsterID: "mago-esqueleto", MonsterName: "Mago Esqueleto", BossName: "Necromante Supremo", Difficulty: "normal", AreaLevel: 60, RequiredLevel: 56},

	// Templo de Carneton (Nível Mínimo 25)
	{GateID: "templo-carneton", GateName: "Templo de Carneton", TierMin: 61, TierMax: 65, MonsterID: "senhor-vampiro", MonsterName: "Senhor Vampiro", BossName: "Lorde Vampiro", Difficulty: "normal", AreaLevel: 65, RequiredLevel: 61},
	{GateID: "templo-carneton", GateName: "Templo de Carneton", TierMin: 66, TierMax: 70, MonsterID: "troll-cavernas", MonsterName: "Troll das Cavernas", BossName: "Troll Ancião", Difficulty: "normal", AreaLevel: 70, RequiredLevel: 66},

	// --- Dificuldade Difícil (Níveis 71 a 140) ---
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 1, TierMax: 5, MonsterID: "rato-ruina", MonsterName: "Rato da Ruína", Difficulty: "dificil", AreaLevel: 75, RequiredLevel: 71},
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 6, TierMax: 10, MonsterID: "morcego", MonsterName: "Morcego Sombrio", Difficulty: "dificil", AreaLevel: 80, RequiredLevel: 76},
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 11, TierMax: 15, MonsterID: "goblin", MonsterName: "Goblin", Difficulty: "dificil", AreaLevel: 85, RequiredLevel: 81},
	{GateID: "blue-gate", GateName: "Blue Gate", TierMin: 16, TierMax: 20, MonsterID: "esqueleto-guerreiro", MonsterName: "Esqueleto Guerreiro", Difficulty: "dificil", AreaLevel: 90, RequiredLevel: 86},
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 21, TierMax: 25, MonsterID: "orc-jovem", MonsterName: "Orc Jovem", Difficulty: "dificil", AreaLevel: 95, RequiredLevel: 91},
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 26, TierMax: 30, MonsterID: "zumbi-faminto", MonsterName: "Zumbi Faminto", Difficulty: "dificil", AreaLevel: 100, RequiredLevel: 96},
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 31, TierMax: 35, MonsterID: "slime-corrosivo", MonsterName: "Slime Corrosivo", Difficulty: "dificil", AreaLevel: 105, RequiredLevel: 101},
	{GateID: "castelo-demonios", GateName: "Castelo dos Demônios", TierMin: 36, TierMax: 40, MonsterID: "aranha-pedra", MonsterName: "Aranha de Pedra", Difficulty: "dificil", AreaLevel: 110, RequiredLevel: 106},
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 41, TierMax: 45, MonsterID: "golem-barro", MonsterName: "Golem de Barro", Difficulty: "dificil", AreaLevel: 115, RequiredLevel: 111},
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 46, TierMax: 50, MonsterID: "espirito-errante", MonsterName: "Espírito Errante", Difficulty: "dificil", AreaLevel: 120, RequiredLevel: 116},
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 51, TierMax: 55, MonsterID: "cavaleiro-morto", MonsterName: "Cavaleiro Morto", Difficulty: "dificil", AreaLevel: 125, RequiredLevel: 121},
	{GateID: "ilha-jeju", GateName: "Ilha Jeju", TierMin: 56, TierMax: 60, MonsterID: "mago-esqueleto", MonsterName: "Mago Esqueleto", Difficulty: "dificil", AreaLevel: 130, RequiredLevel: 126},
	{GateID: "templo-carneton", GateName: "Templo de Carneton", TierMin: 61, TierMax: 65, MonsterID: "senhor-vampiro", MonsterName: "Senhor Vampiro", Difficulty: "dificil", AreaLevel: 135, RequiredLevel: 131},
	{GateID: "templo-carneton", GateName: "Templo de Carneton", TierMin: 66, TierMax: 70, MonsterID: "troll-cavernas", MonsterName: "Troll das Cavernas", Difficulty: "dificil", AreaLevel: 140, RequiredLevel: 136},
}

// GetBestHuntForLevel calcula a caçada com a melhor taxa de XP/h sem penalidade para o nível
func GetBestHuntForLevel(level int) HuntTier {
	if level <= 0 {
		return CatalogTiers[0]
	}

	uLevel := uint32(level)
	var best HuntTier
	found := false

	for _, tier := range CatalogTiers {
		// Verifica se o personagem preenche o nível requerido
		if uLevel >= tier.RequiredLevel {
			// Prioriza maior AreaLevel acessível
			if !found || tier.AreaLevel > best.AreaLevel {
				best = tier
				found = true
			}
		}
	}

	if !found {
		return CatalogTiers[0]
	}
	return best
}

// GetHuntName retorna uma descrição amigável da caçada
func GetHuntName(gateID string, tierMin uint32) string {
	for _, tier := range CatalogTiers {
		if tier.GateID == gateID && tier.TierMin == tierMin {
			return fmt.Sprintf("%s (%s Nv %d)", tier.GateName, tier.MonsterName, tier.AreaLevel)
		}
	}
	if gateID != "" {
		return fmt.Sprintf("%s [T%d]", gateID, tierMin)
	}
	return "Desconhecido"
}

// GetNextCodexHuntForLevel percorre os monstros acessíveis para o nível da party
// e seleciona o primeiro monstro que ainda não atingiu a meta de abates do Codex (Modo Completista).
func GetNextCodexHuntForLevel(level int, codexKills map[string]uint64, targetMilestone uint64) (HuntTier, bool, int, int) {
	if level <= 0 {
		return CatalogTiers[0], false, 0, 0
	}
	uLevel := uint32(level)
	totalMonsters := 0
	completedMonsters := 0

	var pendingTier HuntTier
	foundPending := false

	// Analisa do tier mais baixo para o mais alto dentro da dificuldade normal liberada
	for _, tier := range CatalogTiers {
		if tier.Difficulty == "normal" && uLevel >= tier.RequiredLevel {
			totalMonsters++
			kills := codexKills[tier.MonsterID]
			if kills >= targetMilestone {
				completedMonsters++
			} else if !foundPending {
				pendingTier = tier
				foundPending = true
			}
		}
	}

	if foundPending {
		return pendingTier, true, completedMonsters, totalMonsters
	}
	// Se todos os monstros liberados já concluíram a meta, foca na caçada de maior nível/XP
	return GetBestHuntForLevel(level), false, completedMonsters, totalMonsters
}
