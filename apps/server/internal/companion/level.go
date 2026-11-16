package companion

// LevelInfo describes the level reached by a bond value. Next is the bond
// threshold of the following level; for the highest level it is the level's
// own threshold.
type LevelInfo struct {
	Level int
	Name  string
	Next  int
}

type levelRule struct {
	level     int
	name      string
	threshold int
}

// levelRules mirrors the level table in the companion design document.
var levelRules = []levelRule{
	{0, "陌生", 0},
	{1, "认识", 20},
	{2, "熟悉", 50},
	{3, "搭档", 100},
	{4, "在意", 170},
	{5, "傲娇", 260},
	{6, "只对你", 380},
}

// LevelForBond returns the companion level and title for a bond value.
// Negative bonds fall back to level 0.
func LevelForBond(bond int) LevelInfo {
	idx := 0
	for i, rule := range levelRules {
		if bond >= rule.threshold {
			idx = i
		}
	}
	info := LevelInfo{
		Level: levelRules[idx].level,
		Name:  levelRules[idx].name,
		Next:  levelRules[idx].threshold,
	}
	if idx+1 < len(levelRules) {
		info.Next = levelRules[idx+1].threshold
	}
	return info
}
