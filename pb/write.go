package mcpb

import (
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// WriteLegacyServerListPingResponse writes a legacy server list ping response
// The response format is: 0xFF (packet ID) + response string length (short) + response string (UTF-16BE)
// The response string format is: §1\x00protocol\x00version\x00motd\x00currentPlayers\x00maxPlayers
func WriteLegacyServerListPingResponse(writer io.Writer, protocolVersion int, serverVersion string, motd string, currentPlayers int, maxPlayers int) error {
	// Build the response string - format: §1[NULL]protocol[NULL]version[NULL]motd[NULL]currentPlayers[NULL]maxPlayers
	responseStr := "§1\x00" +
		string(rune(protocolVersion)) + "\x00" +
		serverVersion + "\x00" +
		motd + "\x00" +
		fmt.Sprintf("%d", currentPlayers) + "\x00" +
		fmt.Sprintf("%d", maxPlayers)

	// Convert to UTF-16BE
	utf16beBytes, _, err := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewEncoder(), []byte(responseStr))
	if err != nil {
		return err
	}

	// Write packet ID (0xFF)
	if err := WriteByte(writer, 0xFF); err != nil {
		return err
	}

	// Write string length (in UTF-16 characters, not bytes)
	stringLengthInChars := len(utf16beBytes) / 2
	if err := WriteUnsignedShort(writer, uint16(stringLengthInChars)); err != nil {
		return err
	}

	// Write the UTF-16BE string
	_, err = writer.Write(utf16beBytes)
	return err
}

// WriteByte writes a single byte
func WriteByte(writer io.Writer, value byte) error {
	_, err := writer.Write([]byte{value})
	return err
}

// WriteUnsignedShort writes a 16-bit unsigned integer in big-endian format
func WriteUnsignedShort(writer io.Writer, value uint16) error {
	return binary.Write(writer, binary.BigEndian, value)
}

// WriteUnsignedInt writes a 32-bit unsigned integer in big-endian format
func WriteUnsignedInt(writer io.Writer, value uint32) error {
	return binary.Write(writer, binary.BigEndian, value)
}

// WriteVarInt writes a variable-length integer
func WriteVarInt(writer io.Writer, value int) error {
	for {
		temp := byte(value & 0x7F)
		value >>= 7
		if value != 0 {
			temp |= 0x80
		}
		if err := WriteByte(writer, temp); err != nil {
			return err
		}
		if value == 0 {
			break
		}
	}
	return nil
}

// WriteString writes a string with VarInt length prefix
func WriteString(writer io.Writer, value string) error {
	if err := WriteVarInt(writer, len(value)); err != nil {
		return err
	}
	_, err := writer.Write([]byte(value))
	return err
}

// WriteLoginDisconnect writes a login disconnect packet with the given reason
func WriteLoginDisconnect(writer io.Writer, reason string) error {
	// Create JSON chat component for the disconnect message
	jsonMessage := fmt.Sprintf(`{"text":"%s","color":"red"}`, reason)

	// Calculate packet length (PacketID + JSON message length)
	packetData := []byte{}

	// Write packet ID (0x00 for Login Disconnect)
	packetIdData := []byte{}
	if err := writeVarIntToBuffer(&packetIdData, 0x00); err != nil {
		return err
	}
	packetData = append(packetData, packetIdData...)

	// Write JSON message
	jsonData := []byte{}
	if err := writeStringToBuffer(&jsonData, jsonMessage); err != nil {
		return err
	}
	packetData = append(packetData, jsonData...)

	// Write packet length
	if err := WriteVarInt(writer, len(packetData)); err != nil {
		return err
	}

	// Write packet data
	_, err := writer.Write(packetData)
	return err
}

// Helper function to write VarInt to a buffer
func writeVarIntToBuffer(buffer *[]byte, value int) error {
	for {
		temp := byte(value & 0x7F)
		value >>= 7
		if value != 0 {
			temp |= 0x80
		}
		*buffer = append(*buffer, temp)
		if value == 0 {
			break
		}
	}
	return nil
}

// Helper function to write string to a buffer
func writeStringToBuffer(buffer *[]byte, value string) error {
	if err := writeVarIntToBuffer(buffer, len(value)); err != nil {
		return err
	}
	*buffer = append(*buffer, []byte(value)...)
	return nil
}
