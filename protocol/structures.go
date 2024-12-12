package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func ReadVarInt(r io.Reader) (int, error) {
	b := make([]byte, 1)
	var read uint = 0

	res := 0

	for read <= 5 {
		n, err := r.Read(b)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}

		val := b[0] & 0x7F
		res |= int(val) << (7 * read)

		read++

		if b[0]&0x80 == 0 {
			return res, nil
		}

	}
	return 0, errors.New("VarInt is too big")
}

func ReadUTF16BEString(reader io.Reader, symbolLen uint16) (string, error) {
	bsUtf16be := make([]byte, symbolLen*2)

	_, err := io.ReadFull(reader, bsUtf16be)
	if err != nil {
		return "", err
	}

	result, _, err := transform.Bytes(unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder(), bsUtf16be)
	if err != nil {
		return "", err
	}

	return string(result), nil
}

func ReadByte(reader io.Reader) (byte, error) {
	buf := make([]byte, 1)
	_, err := reader.Read(buf)
	if err != nil {
		return 0, err
	} else {
		return buf[0], nil
	}
}

func ReadUnsignedShort(reader io.Reader) (uint16, error) {
	var value uint16
	err := binary.Read(reader, binary.BigEndian, &value)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func ReadUnsignedInt(reader io.Reader) (uint32, error) {
	var value uint32
	err := binary.Read(reader, binary.BigEndian, &value)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func ReadString(reader io.Reader) (string, error) {
	length, err := ReadVarInt(reader)
	if err != nil {
		return "", err
	}

	b := make([]byte, 1)
	var strBuilder strings.Builder
	for i := 0; i < length; i++ {
		n, err := reader.Read(b)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		strBuilder.WriteByte(b[0])
	}

	return strBuilder.String(), nil
}
