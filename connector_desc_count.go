package main

import (
	"regexp"
	"strconv"
	"strings"
)

// A parts-list description often states how many positions a connector has ("CONNECTOR, 37 POS", "RECEPTACLE
// 15 P"). That is the drawing's own statement, so it is used when the part number is not otherwise known.
var (
	reDescConnectorWord = regexp.MustCompile(`(?i)\b(?:CONNECTOR|PLUG|RECEPTACLE|HEADER|SOCKET|HOUSING|JACK|D-?SUB)\b`)
	reDescPositionCount = regexp.MustCompile(`(?i)\b(\d{1,3})\s*-?\s*(?:POSITIONS?|POS|PINS?|WAYS?|CAVITIES|CAVITY|CKTS?|CIRCUITS?|CONTACTS?|P)\b`)
)

// positionsFromDescription returns the one position count a connector description states. Two different
// counts (or none) give nothing: the description is then not a reliable statement.
func positionsFromDescription(desc string) (int, bool) {
	if !reDescConnectorWord.MatchString(desc) {
		return 0, false
	}
	found := map[int]bool{}
	for _, m := range reDescPositionCount.FindAllStringSubmatch(desc, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 2 || n > 500 {
			continue
		}
		found[n] = true
	}
	if len(found) != 1 {
		return 0, false
	}
	for n := range found {
		return n, true
	}
	return 0, false
}

const descCountStatus = "COUNT STATED IN THE PARTS LIST DESCRIPTION; NOT VERIFIED, LAYOUT UNKNOWN"

func descCountLayout(n int) string { return strings.TrimSpace(descCountStatus) + " (" + strconv.Itoa(n) + " positions)" }
