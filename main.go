package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/blockrouter/ad"
	"github.com/blockrouter/db"
	mcpb "github.com/blockrouter/pb"
)

var dbLocation string = "mongodb://root:safe()Password@cachy:27017"

func handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// Set read deadline for initial packet reading (30 seconds)
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	inspectBuffer := new(bytes.Buffer)
	inspectReader := io.TeeReader(conn, inspectBuffer)
	bufferedReader := bufio.NewReader(inspectReader)

	receivedPacket, err := mcpb.ReadPacket(bufferedReader, conn.RemoteAddr(), mcpb.StateHandshaking)
	if err != nil {
		if err == io.EOF {
			// Client disconnected during handshake, this is normal
			log.Printf("Client %v disconnected during handshake", conn.RemoteAddr())
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// Connection timeout
			log.Printf("Timeout reading handshake packet from %v", conn.RemoteAddr())
		} else {
			log.Printf("Error reading packet from %v: %v", conn.RemoteAddr(), err)
		}
		return
	}

	switch receivedPacket.PacketID {
	case mcpb.PacketIdHandshake:
		hs, err := mcpb.DecodeHandshake(receivedPacket.Data)
		if err != nil {
			log.Printf("Error decoding handshake from %v: %v", conn.RemoteAddr(), err)
			return
		}

		server, err := database.FindServerAddrBySubdomain(hs.ServerAddress)
		if err != nil {
			panic(err)
		}
		if server == nil {
			addr := strings.Split(hs.ServerAddress, ".")[0]
			writeCustomMotdResponse(conn, bufferedReader, "§c• Not found", "§7Server §b"+addr+"§7 not found."+ad.ChooseAd())
			conn.Close()
			return
		}

		switch server.Status {
		case "Running":
			// Forward all to the backend server so it can respond with real-time data
			connectToBackend(ctx, conn, server, inspectBuffer)
		case "Stopped":
			// Handle offline status request
			switch hs.NextState {
			case mcpb.StateStatus:
				motd := server.GameConfig.MessageofTheDay
				if motd == "" {
					motd = "A Minecraft Server"
				}
				// writeCustomMotdResponse(conn, server, "§c• Offline", fmt.Sprintf("§9%s§7 is offline.\n", server.Name)+ad.ChooseAd())
				writeCustomMotdResponse(conn, bufferedReader, "§c• Offline", motd+ad.ChooseAd())
			case mcpb.StateLogin:
				// Read the login start packet to get the username
				loginPacket, err := mcpb.ReadPacket(bufferedReader, conn.RemoteAddr(), mcpb.StateLogin)
				if err != nil {
					if err == io.EOF {
						log.Printf("Client %v disconnected during login", conn.RemoteAddr())
					} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						log.Printf("Timeout reading login packet from %v", conn.RemoteAddr())
					} else {
						log.Printf("Error reading login packet from %v: %v", conn.RemoteAddr(), err)
					}
					return
				}

				if loginPacket.PacketID != mcpb.PacketIdLogin {
					log.Printf("Unexpected packet ID %d from %v during login", loginPacket.PacketID, conn.RemoteAddr())
					return
				}

				loginStart, err := mcpb.DecodeLoginStart(loginPacket.Data)
				if err != nil {
					log.Printf("Error decoding login start from %v: %v", conn.RemoteAddr(), err)
					return
				}

				// Check if Eco mode is enabled and if the player can start the server
				enabled := server.EcoConfig.Enabled && server.EcoConfig.StartWhenJoined
				if enabled {
					// Check if the username is in the whitelist (if whitelist exists)
					canStart := len(server.EcoConfig.StartWhenJoinWhitelist) == 0
					if !canStart {
						canStart = slices.Contains(server.EcoConfig.StartWhenJoinWhitelist, loginStart.Name)
					}

					if canStart {
						mcpb.WriteLoginDisconnect(conn, fmt.Sprintf("§9%s §7is starting up...", server.Name)+"\n§7It may take a moment. Refresh the server list until it appears online.")
						log.Printf("Player %s triggered server start for '%s'", loginStart.Name, server.Name)
						return
					}
				}

				// Otherwise, send a disconnect message
				err = mcpb.WriteLoginDisconnect(conn, fmt.Sprintf("§7Server §9%s §7is currently offline", server.Name))
				if err != nil {
					log.Printf("Error writing login disconnect to %v: %v", conn.RemoteAddr(), err)
				} else {
					log.Printf("Disconnected %s (%v) from stopped server '%s'", loginStart.Name, conn.RemoteAddr(), server.Name)
				}
			default:
				// Handle unexpected state
				log.Printf("Unexpected handshake state %v for server %s", hs.NextState, server.Name)
				conn.Close()
			}

		case "Starting":
			// Handle starting server status
			switch hs.NextState {
			case mcpb.StateStatus:
				motd := server.GameConfig.MessageofTheDay
				if motd == "" {
					motd = "A Minecraft Server"
				}
				writeCustomMotdResponse(conn, bufferedReader, "§6⏳ Starting", motd+ad.ChooseAd())
			case mcpb.StateLogin:
				err := mcpb.WriteLoginDisconnect(conn, fmt.Sprintf("Server '%s' is currently starting", server.Name))
				if err != nil {
					log.Printf("Error writing login disconnect to %v: %v", conn.RemoteAddr(), err)
				} else {
					log.Printf("Disconnected %v from starting server '%s'", conn.RemoteAddr(), server.Name)
				}
			default:
				// Handle unexpected state
				log.Printf("Unexpected handshake state %v for starting server %s", hs.NextState, server.Name)
				conn.Close()
			}
		default:
			// Handle unknown status
			writeCustomMotdResponse(conn, bufferedReader, "§cUnknown Status", "§cThis server is in an unknown state.")
		}

	// Handle legacy server list ping, which is a special case
	case mcpb.PacketIdLegacyServerListPing:
		fmt.Println("Received legacy server list ping")
		hs, ok := receivedPacket.Data.(*mcpb.LegacyServerListPing)

		if !ok {
			log.Printf("Error decoding legacy server list ping from %v: expected *mcpb.LegacyServerListPing, got %T", conn.RemoteAddr(), receivedPacket.Data)
			return
		}

		server, err := database.FindServerAddrBySubdomain(hs.ServerAddress)
		if err != nil {
			panic(err)
		}
		if server == nil {
			addr := strings.Split(hs.ServerAddress, ".")[0]
			err := mcpb.WriteLegacyServerListPingResponse(
				conn,
				9999,
				"§c• Not found", "§7Server §b"+addr+"§7 not found."+ad.ChooseAd(),
				0, // Current players
				0, // Max players
			)
			if err != nil {
				log.Printf("Error writing legacy server list ping response to %v: %v", conn.RemoteAddr(), err)
			}
			return
		}

		switch server.Status {
		case "Running":
			// Forward the legacy ping to the backend server so it can respond with real-time data
			connectToBackend(ctx, conn, server, inspectBuffer)

		case "Stopped":
			err := mcpb.WriteLegacyServerListPingResponse(
				conn,
				hs.ProtocolVersion,
				server.GameConfig.Version,
				fmt.Sprintf("§c%s§r - Server is currently offline", server.Name),
				0, // Current players
				0, // Max players
			)
			if err != nil {
				log.Printf("Error writing legacy server list ping response to %v: %v", conn.RemoteAddr(), err)
			}

		case "Starting":
			log.Printf("Received legacy server list ping for server %s with unknown status: %s", server.Name, server.Status)
			err := mcpb.WriteLegacyServerListPingResponse(
				conn,
				hs.ProtocolVersion,
				server.GameConfig.Version,
				fmt.Sprintf("§c%s§r - Server is currently starting!", server.Name),
				0, // Current players
				0, // Max players
			)
			if err != nil {
				log.Printf("Error writing legacy server list ping response to %v: %v", conn.RemoteAddr(), err)
			}
		}
	}

}

func connectToBackend(ctx context.Context, conn net.Conn, server *db.Server, inspectBuffer *bytes.Buffer) {
	// Create a context for the backend connection with timeout
	dialCtx, dialCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dialCancel()

	var dialer net.Dialer
	beConn, err := dialer.DialContext(dialCtx, "tcp", server.Endpoint)
	if err != nil {
		log.Printf("Error connecting to backend server: %v", err)
		return
	}
	defer beConn.Close()

	// Write the captured handshake data to backend
	if _, err := beConn.Write(inspectBuffer.Bytes()); err != nil {
		log.Printf("Error writing to backend server: %v", err)
		return
	}

	fmt.Printf("Connected to backend server: %v\n", beConn.RemoteAddr())

	// Create a context for the proxy session that can be cancelled
	proxyCtx, proxyCancel := context.WithCancel(ctx)
	defer proxyCancel()

	var wg sync.WaitGroup

	// Client to backend
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer proxyCancel() // Cancel the other goroutine when this one ends
		copyWithContext(proxyCtx, beConn, conn, "client->backend")
	}()

	// Backend to client
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer proxyCancel() // Cancel the other goroutine when this one ends
		copyWithContext(proxyCtx, conn, beConn, "backend->client")
	}()

	// Wait for either goroutine to finish or context to be cancelled
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Printf("Proxy session ended for %v\n", conn.RemoteAddr())
	case <-proxyCtx.Done():
		fmt.Printf("Proxy session cancelled for %v: %v\n", conn.RemoteAddr(), proxyCtx.Err())
	}
}

// copyWithContext copies data from src to dst while respecting context cancellation
func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, direction string) {
	// Create a buffer for copying
	buf := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Copy cancelled (%s): %v", direction, ctx.Err())
			return
		default:
		}

		// Set read timeout if possible
		if conn, ok := src.(net.Conn); ok {
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		}

		n, err := src.Read(buf)
		if n > 0 {
			// Set write timeout if possible
			if conn, ok := dst.(net.Conn); ok {
				conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
			}

			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				log.Printf("Write error (%s): %v", direction, writeErr)
				return
			}
		}

		if err != nil {
			if err != io.EOF {
				log.Printf("Read error (%s): %v", direction, err)
			}
			return
		}
	}
}

var database *db.Db

func main() {
	var err error
	if loc := os.Getenv("DB_LOCATION"); loc != "" {
		dbLocation = loc
		fmt.Println("Using DB_LOCATION from environment:", dbLocation)
	}

	database, err = db.NewDb(dbLocation, "serverpanel")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	// Create a context that can be cancelled for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown on SIGINT/SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, stopping server...")
		cancel()
	}()

	l, err := net.Listen("tcp", ":25565")
	if err != nil {
		log.Fatalf("Failed to listen on port 25565: %v", err)
	}
	defer l.Close()

	log.Println("BlockRouter started on :25565")

	// Track active connections
	var wg sync.WaitGroup
	var activeConnections atomic.Int32

	go func() {
		for {
			time.Sleep(5 * time.Second)
			fmt.Println("Active connections:", activeConnections.Load())
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("Server shutting down...")
			// Wait for all connections to finish
			wg.Wait()
			return
		default:
		}

		// Set accept timeout to allow checking for context cancellation
		if tcpListener, ok := l.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}

		conn, err := l.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected, continue to check context
				continue
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		log.Printf("New connection established: %v", conn.RemoteAddr())

		wg.Add(1)
		activeConnections.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			defer activeConnections.Add(-1)
			handleConnection(ctx, conn)
		}(conn)
	}
}

func writeCustomMotdResponse(conn net.Conn, reader *bufio.Reader, statusText, motd string) {
	// Use the existing buffered reader instead of creating a new one

	// Set a timeout for status packet reading (15 seconds)
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	for {
		// Read the next packet in status state
		packet, err := mcpb.ReadPacket(reader, conn.RemoteAddr(), mcpb.StateStatus)
		if err != nil {
			if err == io.EOF {
				// Client disconnected, this is normal
				log.Printf("Client %v disconnected during status request", conn.RemoteAddr())
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Connection timeout
				log.Printf("Timeout reading status packet from %v", conn.RemoteAddr())
			} else {
				log.Printf("Error reading status packet from %v: %v", conn.RemoteAddr(), err)
			}
			return
		}

		switch packet.PacketID {
		case 0x00: // Status Request
			// Send status response
			statusJSON := fmt.Sprintf(`{
				"version": {
					"name": "%s",
					"protocol": 9999
				},
				"players": {
					"max": 0,
					"online": 0,
					"sample": [
						{
							"name": "§8§l======================================",
							"id": "00000000-0000-0000-0000-000000000000"
						},
						{
							"name": "",
							"id": "00000000-0000-0000-0000-000000000000"
						},
						{
							"name": "§7Get your server at §bPlexHost",
							"id": "00000000-0000-0000-0000-000000000000"
						},
						{
							"name": "§7Up to §b§u100 hours §7of gameplay for a single dollar",
							"id": "00000000-0000-0000-0000-000000000000"
						},
						{
							"name": "§7Sign up and get §b50 credits§7 for free at §bPlexhost.com§7!",
							"id": "00000000-0000-0000-0000-000000000000"
						},
						{
							"name": "",
							"id": "00000000-0000-0000-0000-000000000000"
						},
						{
							"name": "§8§l======================================",
							"id": "00000000-0000-0000-0000-000000000000"
						}
					]
				},
				"description": {
					"text": "%s"
				}
			}`, statusText, motd)

			// Write status response packet
			var buf bytes.Buffer
			if err := mcpb.WriteVarInt(&buf, 0x00); err != nil { // Packet ID
				log.Printf("Error writing packet ID: %v", err)
				return
			}
			if err := mcpb.WriteString(&buf, statusJSON); err != nil { // JSON payload
				log.Printf("Error writing JSON: %v", err)
				return
			}

			// Send the packet with length prefix
			var finalBuf bytes.Buffer
			if err := mcpb.WriteVarInt(&finalBuf, buf.Len()); err != nil { // Packet length
				log.Printf("Error writing packet length: %v", err)
				return
			}
			finalBuf.Write(buf.Bytes())

			if _, err := conn.Write(finalBuf.Bytes()); err != nil {
				log.Printf("Error sending status response: %v", err)
				return
			}

		case 0x01: // Ping Request
			// Read the ping payload (8 bytes)
			data := packet.Data.([]byte)
			if len(data) < 8 {
				log.Printf("Invalid ping packet length: %d", len(data))
				return
			}

			// Send pong response with same payload
			var buf bytes.Buffer
			if err := mcpb.WriteVarInt(&buf, 0x01); err != nil { // Packet ID
				log.Printf("Error writing pong packet ID: %v", err)
				return
			}
			buf.Write(data[:8]) // Echo the ping payload

			// Send the packet with length prefix
			var finalBuf bytes.Buffer
			if err := mcpb.WriteVarInt(&finalBuf, buf.Len()); err != nil { // Packet length
				log.Printf("Error writing pong packet length: %v", err)
				return
			}
			finalBuf.Write(buf.Bytes())

			if _, err := conn.Write(finalBuf.Bytes()); err != nil {
				log.Printf("Error sending pong response: %v", err)
				return
			}

			// After pong, the client typically closes the connection
			return

		default:
			log.Printf("Unknown status packet ID: 0x%02X", packet.PacketID)
			return
		}
	}
}
