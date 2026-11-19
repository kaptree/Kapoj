package companion

// levelOutfitUnlocks maps bond thresholds to the outfit unlocked at that
// level. The order is also the display order in the wardrobe.
var levelOutfitUnlocks = []struct {
	bond int
	id   string
}{
	{0, "outfit-1"},
	{20, "outfit-2"},
	{50, "outfit-3"},
	{170, "outfit-4"},
	{380, "outfit-5"},
}

// LevelUnlocks returns the outfit ids available at a bond value.
func LevelUnlocks(bond int) []string {
	out := make([]string, 0, len(levelOutfitUnlocks))
	for _, unlock := range levelOutfitUnlocks {
		if bond >= unlock.bond {
			out = append(out, unlock.id)
		}
	}
	return out
}

// mergeUnlockedSkins combines the stored legacy list with the level-based
// unlocks. Level outfits stay in 1..5 order; stored extras follow, deduped.
func mergeUnlockedSkins(stored []string, bond int) []string {
	seen := make(map[string]struct{}, len(stored)+len(levelOutfitUnlocks))
	out := make([]string, 0, len(stored)+len(levelOutfitUnlocks))
	for _, id := range LevelUnlocks(bond) {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range stored {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func skinAvailable(skins []string, id string) bool {
	for _, skin := range skins {
		if skin == id {
			return true
		}
	}
	return false
}
