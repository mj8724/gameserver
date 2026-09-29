package pz

import "testing"

func TestParseA2SInfo(t *testing.T) {
	// Synthesize a valid A2S_INFO response with the expected field layout.
	var packet []byte
	packet = append(packet, 0xFF, 0xFF, 0xFF, 0xFF, 0x49) // header + 0x49
	packet = append(packet, 0x11)                         // protocol
	packet = append(packet, []byte("servertest\x00")...)
	packet = append(packet, []byte("Muldraugh, KY\x00")...)
	packet = append(packet, []byte("Root\x00")...)
	packet = append(packet, []byte("Project Zomboid\x00")...)
	packet = append(packet, 0x5E, 0xCF, 0x05, 0x00) // appid 380870
	packet = append(packet, 0x03, 0x31, 0x00)       // players 3, max 49, bots 0
	packet = append(packet, 'd', 'l', 0x00)         // dedi, linux, public
	packet = append(packet, []byte("42.21\x00")...)

	info, err := parseA2SInfo(packet)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Name != "servertest" || info.Map != "Muldraugh, KY" {
		t.Fatalf("name/map wrong: %+v", info)
	}
	if info.Players != 3 || info.Max != 49 {
		t.Fatalf("players wrong: %+v", info)
	}
}

func TestParseA2SInfoRejectsGarbage(t *testing.T) {
	if _, err := parseA2SInfo([]byte{0x01, 0x02, 0x03}); err == nil {
		t.Fatal("short garbage must be rejected")
	}
	if _, err := parseA2SInfo([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x50, 0x00}); err == nil {
		t.Fatal("non-0x49 response must be rejected")
	}
}
