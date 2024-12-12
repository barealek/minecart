package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/charmbracelet/log"
)

func ReadPacket(r io.Reader, addr net.Addr, s State) (*Packet, error) {
	log.Debug("reading packet", "addr", addr)

	if s == StateHandshaking {
		bufr := bufio.NewReader(r)
		data, err := bufr.Peek(1)
		if err != nil {
			return nil, err
		}

		if data[0] == PacketLegacyServerPing {
			return ReadLegacyServerPing(bufr, addr)
		} else {
			r = bufr
		}
	}

	frame, err := ReadFrame(r, addr)
	if err != nil {
		return nil, err
	}

	packet := &Packet{Length: frame.Length}

	rem := bytes.NewBuffer(frame.Payload)

	packet.PacketID, err = ReadVarInt(rem)
	if err != nil {
		return nil, err
	}

	packet.Data = rem.Bytes()

	log.Debug("read packet", "packet", packet)

	return packet, nil
}

func ReadLegacyServerPing(r *bufio.Reader, addr net.Addr) (*Packet, error) {
	log.Debug("reading legacy server list ping", "client", addr)

	packetid, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if packetid != PacketLegacyServerPing {
		return nil, fmt.Errorf("expected legacy server list ping, got %x", packetid)
	}

	p, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	if p != 0x01 {
		return nil, fmt.Errorf("expected legacy server payload=1, got %x", p)
	}

	packetidForPluginMsg, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	if packetidForPluginMsg != 0xFA {
		return nil, fmt.Errorf("expected packetidForPluginMsg == 0xFA, got %x", packetidForPluginMsg)
	}

	messageNameShortLen, err := ReadUnsignedShort(r)
	if err != nil {
		return nil, err
	}
	if messageNameShortLen != 11 {
		return nil, fmt.Errorf("expected messageNameShortLen=11 from legacy server listing ping, got %d", messageNameShortLen)
	}

	messageName, err := ReadUTF16BEString(r, messageNameShortLen)
	if err != nil {
		return nil, err
	}
	if messageName != "MC|PingHost" {
		return nil, fmt.Errorf("expected messageName=MC|PingHost, got %s", messageName)
	}

	remainingLen, err := ReadUnsignedShort(r)
	if err != nil {
		return nil, err
	}
	remainingReader := io.LimitReader(r, int64(remainingLen))

	protocolVersion, err := ReadByte(remainingReader)
	if err != nil {
		return nil, err
	}

	hostnameLen, err := ReadUnsignedShort(remainingReader)
	if err != nil {
		return nil, err
	}
	hostname, err := ReadUTF16BEString(remainingReader, hostnameLen)
	if err != nil {
		return nil, err
	}

	port, err := ReadUnsignedInt(remainingReader)
	if err != nil {
		return nil, err
	}

	return &Packet{
		PacketID: PacketLegacyServerPing,
		Length:   0,
		Data: &LegacyServerPing{
			ProtocolVersion: int(protocolVersion),
			ServerAddress:   hostname,
			ServerPort:      uint16(port),
		},
	}, nil
}

func ReadFrame(r io.Reader, addr net.Addr) (*Frame, error) {
	log.Debug("reading frame", "client", addr)

	var (
		err   error
		frame = new(Frame)
	)

	frame.Length, err = ReadVarInt(r)
	if err != nil {
		return nil, err
	}

	if frame.Length > (2^21)-1 {
		return nil, fmt.Errorf("frame length %d too large", frame.Length)
	}

	frame.Payload = make([]byte, frame.Length)

	total := 0

	for total < frame.Length {
		readInto := frame.Payload[total:]

		n, err := r.Read(readInto)
		if err != nil {
			return nil, err
		}
		total += n

		if n == 0 {

			log.Warn("no progress on frame", "frame", frame)

			time.Sleep(100 * time.Millisecond)
		}
	}

	log.Debug("read frame", "frame", frame, "client", addr)

	return frame, nil
}

func ReadHandshake(d any) (*Handshake, error) {
	dataAsBytes, ok := d.([]byte)
	if !ok {
		return nil, fmt.Errorf("data is not expected byte slice")
	}

	handshake := &Handshake{}
	buf := bytes.NewBuffer(dataAsBytes)
	var err error

	handshake.ProtoVersion, err = ReadVarInt(buf)
	if err != nil {
		return nil, err
	}

	handshake.ServerAdress, err = ReadString(buf)
	if err != nil {
		return nil, err
	}

	handshake.ServerPort, err = ReadUnsignedShort(buf)
	if err != nil {
		return nil, err
	}

	handshake.NextState, err = ReadVarInt(buf)
	if err != nil {
		return nil, err
	}

	return handshake, nil
}
