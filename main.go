package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/barealek/minecart/protocol"
	"github.com/charmbracelet/log"
)

func main() {
	db := flag.Bool("debug", false, "")
	flag.Parse()

	if *db {
		log.SetLevel(log.DebugLevel)
	}

	l, err := net.Listen("tcp", ":25565")
	if err != nil {
		panic(err)
	}

	clientConn, err := l.Accept()
	if err != nil {
		panic(err)
	}
	clientAddr := clientConn.RemoteAddr()

	inspectionBuffer := new(bytes.Buffer)
	insReader := io.TeeReader(clientConn, inspectionBuffer)

	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))

	packet, err := protocol.ReadPacket(insReader, clientAddr, 0)
	if err != nil {
		panic(err)
	}

	if packet.PacketID == protocol.PacketLegacyServerPing {
		handshake, ok := packet.Data.(*protocol.LegacyServerPing)
		if !ok {
			log.Warn("packet is not a legacy server ping")
			return
		}
		fmt.Printf("handshake: %v\n", handshake)
		return
	}

	if packet.PacketID == protocol.PacketHandshake {
		handshake, err := protocol.ReadHandshake(packet.Data)
		if err != nil {
			log.Warn("error reading handshake packet", "error", err)
		}

		log.Info("handshake successfully received", "handshake", handshake, "client", clientAddr)
		tekst := fmt.Sprintf("handshake.ServerAdress: %v. Porten er %d\n", handshake.ServerAdress, handshake.ServerPort)
		fmt.Printf("len(tekst): %v\n", len(tekst))
		return
	}

}
