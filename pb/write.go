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
