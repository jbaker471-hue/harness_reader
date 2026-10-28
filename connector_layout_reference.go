package main

import (
	"fmt"
	"strings"
)

// Inventory IDs are separate from physical face coordinates. Only complete
// part-number matches and explicitly reviewed guide rows establish an inventory.
type GuideInventory struct {
	IDs         []string
	Source      string
	Arrangement string
}

func guideInventory(part string) (GuideInventory, bool) {
	m := seriesIII35.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(part)))
	if len(m) == 0 {
		return GuideInventory{}, false
	}
	rows, e := readLibrary("CONNECTOR_LAYOUT_REFERENCE.csv")
	if e != nil {
		return GuideInventory{}, false
	}
	key := m[3] + "35"
	var found *GuideInventory
	for _, r := range rows[1:] {
		if r[0] != "D38999 Series III" || r[1] != key || r[4] != "reviewed-inventory" {
			continue
		}
		ids := strings.Split(r[2], ";")
		seen := map[string]bool{}
		for _, id := range ids {
			if id == "" || strings.TrimSpace(id) != id || seen[id] {
				return GuideInventory{}, false
			}
			seen[id] = true
		}
		candidate := GuideInventory{IDs: ids, Source: fmt.Sprintf("%s; arrangement %s; cavity-ID inventory only", r[3], key), Arrangement: key}
		if found != nil {
			return GuideInventory{}, false
		}
		found = &candidate
	}
	if found == nil {
		return GuideInventory{}, false
	}
	return *found, true
}
