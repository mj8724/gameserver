package pz

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

// A2SInfo carries the Source-engine info fields used by the query surface.
type A2SInfo struct {
	Name    string
	Map     string
	Players int
	Max     int
}

// QueryA2SInfo sends one A2S_INFO request and parses the response. The legacy
// probe validated only the first response byte; this parser extracts the
// server name, map and player counts so the query surface can project them,
// or reports unavailable when the game is not A2S-reachable.
func QueryA2SInfo(ctx context.Context, host string, port int) (A2SInfo, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if port < 1 || port > 65535 {
		return A2SInfo{}, errors.New("invalid A2S port")
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return A2SInfo{}, err
	}
	defer conn.Close()
	// A2S_INFO request: 0xFFFFFFFF 'T' 'S' 'S' 'o' 'u' 'r' 'c' 'e' ' ' 'E' 'n' 'g' 'i' 'n' 'e' ' ' 'Q' 'u' 'e' 'r' 'y' 0x00
	request := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	request = append(request, []byte("TSource Engine Query")...)
	request = append(request, 0x00)
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	}
	if _, err := conn.Write(request); err != nil {
		return A2SInfo{}, err
	}
	buffer := make([]byte, 2048)
	n, err := conn.Read(buffer)
	if err != nil {
		return A2SInfo{}, err
	}
	info, err := parseA2SInfo(buffer[:n])
	if err != nil {
		return A2SInfo{}, err
	}
	return info, nil
}

// parseA2SInfo decodes the A2S_INFO response body:
//
//	0x49 header, protocol byte, name\0, map\0, folder\0, game\0, appid,
//	players byte, maxplayers byte, bots byte, server type, environment,
//	visibility, version\0 (EDF follows; not needed for the query surface).
func parseA2SInfo(packet []byte) (A2SInfo, error) {
	if len(packet) < 5 || packet[4] != 0x49 {
		return A2SInfo{}, errors.New("not an A2S_INFO response")
	}
	body := packet[5:]
	if len(body) < 6 {
		return A2SInfo{}, errors.New("A2S_INFO response too short")
	}
	name, rest, err := cString(body[1:])
	if err != nil {
		return A2SInfo{}, err
	}
	mapName, rest, err := cString(rest)
	if err != nil {
		return A2SInfo{}, err
	}
	_, rest, err = cString(rest) // folder
	if err != nil {
		return A2SInfo{}, err
	}
	_, rest, err = cString(rest) // game
	if err != nil {
		return A2SInfo{}, err
	}
	rest = rest[4:] // appid
	if len(rest) < 6 {
		return A2SInfo{}, errors.New("A2S_INFO player fields missing")
	}
	players := int(rest[0])
	maxPlayers := int(rest[1])
	return A2SInfo{Name: name, Map: mapName, Players: players, Max: maxPlayers}, nil
}

func cString(data []byte) (string, []byte, error) {
	index := bytes.IndexByte(data, 0)
	if index < 0 {
		return "", nil, errors.New("unterminated string in A2S response")
	}
	return string(data[:index]), data[index+1:], nil
}
