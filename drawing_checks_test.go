package main

import (
	"strings"
	"testing"
)

func codes(iss []ViewIssue) string {
	c := []string{}
	for _, i := range iss {
		c = append(c, i.Code)
	}
	return strings.Join(c, ",")
}

func TestColourTypoIsReportedNotGuessed(t *testing.T) {
	a := Analysis{Wires: []WireRow{
		{Page: 6, Connector: "P1", Cavity: "11", Signal: "ETH0_3+", End: "ETH0", EndCavity: "7", EndSignal: "ETH0_3+ (Whtie/Brown)"},
		{Page: 6, Connector: "P1", Cavity: "37", Signal: "ETH0_3-", End: "ETH0", EndCavity: "8", EndSignal: "ETH0_3- (Brown)", Color: "BRN"},
		{Page: 6, Connector: "P1", Cavity: "5", Signal: "A", End: "X1", EndCavity: "1", EndSignal: "A(-)"},
	}}
	iss := drawingWeaknessIssues(&a)
	if codes(iss) != "WIRE_COLOR_NOT_RECOGNIZED" || !strings.Contains(iss[0].Message, "Whtie/Brown") || !strings.Contains(iss[0].Message, "left blank, not guessed") || iss[0].Pages[0] != 6 {
		t.Fatalf("%+v", iss)
	}
}

func TestSignalRenamesAreFlaggedButCrossoversAndColourNotesAreNot(t *testing.T) {
	a := Analysis{Wires: []WireRow{
		// transmit/receive crossover: intended
		{Page: 4, Connector: "P1", Cavity: "66", Signal: "COM2_422_TXD_A(-)", End: "DB9 COM2", EndCavity: "5", EndSignal: "COM2_422_RXD_A(-)"},
		{Page: 4, Connector: "P1", Cavity: "67", Signal: "COM2_422_RXD_A(-)", End: "DB9 COM2", EndCavity: "9", EndSignal: "COM2_422_TXD_A(-)"},
		// same name with a colour note and different punctuation: the same signal
		{Page: 8, Connector: "P1", Cavity: "16", Signal: "AUDIO_GND", End: "AUDIO IN", EndCavity: "GND", EndSignal: "AUDIO GND"},
		{Page: 3, Connector: "P1", Cavity: "6", Signal: "USB_DN1_N", End: "USB_DN1", EndCavity: "2", EndSignal: "USB_DN1_N (White)", Color: "WHT"},
		// a rename
		{Page: 7, Connector: "P1", Cavity: "52", Signal: "PORT1_MDI0+", End: "ETH PORT1", EndCavity: "1", EndSignal: "SL1_ETH3_0+ (White/Orange)", Color: "WHT/ORN"},
		{Page: 7, Connector: "P1", Cavity: "53", Signal: "PORT1_MDI0-", End: "ETH PORT1", EndCavity: "2", EndSignal: "SL1_ETH3_0- (Orange)", Color: "ORN"},
	}}
	iss := drawingWeaknessIssues(&a)
	if codes(iss) != "SIGNAL_NAME_CHANGES" {
		t.Fatalf("%s: %+v", codes(iss), iss)
	}
	m := iss[0].Message
	if !strings.Contains(m, "Sheet 7, to ETH PORT1") || !strings.Contains(m, "2 of 2 wires") || !strings.Contains(m, "PORT1_MDI0+ becomes SL1_ETH3_0+") {
		t.Fatalf("%s", m)
	}
}

func TestPinUsedTwiceIsAQuestionAndNoiseIsAvoided(t *testing.T) {
	a := Analysis{Wires: []WireRow{
		{Page: 2, Connector: "P1", Cavity: "5", End: "J2", EndCavity: "A"},
		{Page: 3, Connector: "P1", Cavity: "5", End: "J3", EndCavity: "B"},
		{Page: 3, Connector: "P1", Cavity: "6", End: "J3", EndCavity: "C"},
		{Page: 3, Connector: "P1", Cavity: "6", End: "J3", EndCavity: "C"}, // the same wire read twice is not a second wire
		{Page: 3, Connector: "UNASSIGNED", Cavity: "1", End: "J9", EndCavity: "1"},
		{Page: 3, Connector: "UNASSIGNED", Cavity: "1", End: "J8", EndCavity: "1"},
	}}
	iss := drawingWeaknessIssues(&a)
	if codes(iss) != "PIN_USED_TWICE" || iss[0].Connector != "P1" || !strings.Contains(iss[0].Message, "P1 pin 5") || !strings.Contains(iss[0].Message, "J2:A, J3:B") || iss[0].Severity != "REVIEW" {
		t.Fatalf("%+v", iss)
	}
	if got := drawingWeaknessIssues(&Analysis{}); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}
