package protocol

import (
	"fmt"
)

type Frame struct {
	Length  int
	Payload []byte
}

const trimLimit = 64

func trimBytes(d []byte) ([]byte, string) {
	if len(d) < trimLimit {
		return d, ""
	}

	return d[:trimLimit], "..."
}

func (f *Frame) String() string {
	trimmed, cont := trimBytes(f.Payload)
	return fmt.Sprintf("Frame{len=%d, payload=%#x%s}", f.Length, trimmed, cont)
}

const (
	PacketHandshake        = 0x00
	PacketLegacyServerPing = 0xFE
)

type Handshake struct {
	ProtoVersion int
	ServerAdress string
	ServerPort   uint16
	NextState    int
}

func (h *Handshake) String() string {
	return fmt.Sprintf("Handshake{addr=%s, port=%d, version=%d}", h.ServerAdress, h.ServerPort, h.ProtoVersion)
}

type LegacyServerPing struct {
	ProtocolVersion int
	ServerAddress   string
	ServerPort      uint16
}

type State int

const (
	StateHandshaking = iota
)

type Packet struct {
	Length   int
	PacketID int
	Data     any
}

func (p *Packet) String() string {
	return fmt.Sprintf("Packet{length: %d, id: %d, data: %#x}", p.Length, p.PacketID, p.Data)
}
