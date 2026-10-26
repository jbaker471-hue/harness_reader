package main

import (
	"fmt"
	"regexp"
	"strings"
)

// ConnectorContact is a manufacturer-catalog proposal, not approval that the
// exact contact was procured, supplied, or qualified for the drawing's wire.
type ConnectorContact struct {
	Part        string `json:"part"`
	Sex         string `json:"sex"`
	ContactSize string `json:"contactSize"`
	Qty         int    `json:"qty"`
	Cycle       string `json:"cycle"`
	Source      string `json:"source"`
	Status      string `json:"status"`
	Warning     string `json:"warning"`
}

type ContactResolution struct {
	ConnectorPart string             `json:"connectorPart"`
	Arrangement   string             `json:"arrangement"`
	PositionCount int                `json:"positionCount"`
	ContactSex    string             `json:"contactSex"`
	Status        string             `json:"status"`
	Source        string             `json:"source"`
	Warning       string             `json:"warning"`
	Contacts      []ConnectorContact `json:"contacts"`
}

var seriesIII35 = regexp.MustCompile(`^D38999/(20|24|26)([A-Z])([A-HJ])35([PSHJAB])([NABCDE])?$`)

// Source: Amphenol Aerospace 38999 catalog, PDF pages 5-6 (insert
// availability, 35 arrangement rows), 31 (TV/CTV contact-to-M39029 mapping),
// and 45 (part-number builder contact type). Do not generalize this table to
// a mixed-insert arrangement, solder, fiber, coax/twinax, PCB or other series.
func ResolveConnectorContacts(part string) ContactResolution {
	p := strings.ToUpper(strings.TrimSpace(part))
	out := ContactResolution{
		ConnectorPart: p,
		Status:        "unresolved",
		Warning:       "No validated connector-to-contact cross-reference for this part number. Consult the connector manufacturer's arrangement/contact table.",
		Contacts:      []ConnectorContact{},
	}
	m := seriesIII35.FindStringSubmatch(p)
	if len(m) == 0 {
		return out
	}
	counts := map[string]int{"A": 6, "B": 13, "C": 22, "D": 37, "E": 55, "F": 66, "G": 79, "H": 100, "J": 128}
	qty := counts[m[3]]
	if qty == 0 {
		return out
	}
	sexCode := m[4]
	label := map[string]string{"P": "PIN", "S": "SOCKET", "H": "PIN", "J": "SOCKET", "A": "PIN", "B": "SOCKET"}[sexCode]
	out.Arrangement = fmt.Sprintf("%d-35", map[string]int{"A": 9, "B": 11, "C": 13, "D": 15, "E": 17, "F": 19, "G": 21, "H": 23, "J": 25}[m[3]])
	out.PositionCount = qty
	out.ContactSex = label
	out.Source = "Amphenol 38999 catalog PDF pp. 5-6 (insert layout), 31 (standard and 1500-cycle M39029), 45 (contact code)"
	if sexCode == "A" || sexCode == "B" {
		out.Status = "less-contacts"
		out.Warning = "Connector code specifies LESS contacts. The connector is supplied without this contact type. Do not assume a contact P/N or crimp tool until the separately procured contact is identified."
		return out
	}
	contact := map[string]string{
		"P": "M39029/58-360", "S": "M39029/56-348",
		"H": "M39029/107-620", "J": "M39029/106-614",
	}[sexCode]
	cycle := "500"
	if sexCode == "H" || sexCode == "J" {
		cycle = "1500"
	}
	out.Status = "manufacturer-catalog-candidate"
	out.Warning = "Manufacturer standard contact reference, not proof of delivered contact, finish, approved wire size, or application-specific tooling. Confirm actual supplied contact and current tooling instructions."
	out.Contacts = append(out.Contacts, ConnectorContact{
		Part: contact, Sex: label, ContactSize: "22D", Qty: qty,
		Cycle: cycle, Source: out.Source,
		Status: "CATALOG CANDIDATE — ENGINEERING REVIEW", Warning: out.Warning,
	})
	return out
}
