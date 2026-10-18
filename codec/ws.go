package codec

import (
	"encoding/binary"
	"errors"
	"io"

	rg "rustygo"
)

type WSOpcode byte

const (
	WSOpContinuation WSOpcode = 0x0
	WSOpText         WSOpcode = 0x1
	WSOpBinary       WSOpcode = 0x2
	WSOpClose        WSOpcode = 0x8
	WSOpPing         WSOpcode = 0x9
	WSOpPong         WSOpcode = 0xA
)

var (
	ErrMalformedWSFrame = errors.New("codec/ws: malformed WebSocket frame")
	ErrControlFrameFrag = errors.New("codec/ws: control frames cannot be fragmented")
)

// WSFrame represents a parsed WebSocket frame whose payload is allocated in an arena Scope.
type WSFrame struct {
	Opcode  WSOpcode
	Fin     bool
	Payload []byte
}

// ReadWSFrame reads and unmasks an RFC 6455 WebSocket frame directly into the arena Scope.
func ReadWSFrame(r io.Reader, scope *rg.Scope) (*WSFrame, error) {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}

	fin := (header[0] & 0x80) != 0
	opcode := WSOpcode(header[0] & 0x0F)
	isMasked := (header[1] & 0x80) != 0
	payloadLen := int64(header[1] & 0x7F)

	if !fin && (opcode >= WSOpClose) {
		return nil, ErrControlFrameFrag
	}

	if payloadLen == 126 {
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
	} else if payloadLen == 127 {
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
	}

	var maskKey [4]byte
	if isMasked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return nil, err
		}
	}

	var payload []byte
	if payloadLen > 0 {
		if scope != nil {
			payload = rg.AllocSlice[byte](scope, int(payloadLen))
		} else {
			payload = make([]byte, payloadLen)
		}

		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}

		if isMasked {
			unmaskVectorized(payload, maskKey)
		}
	}

	return &WSFrame{
		Opcode:  opcode,
		Fin:     fin,
		Payload: payload,
	}, nil
}

// unmaskVectorized applies the 4-byte mask key using 64-bit word XOR fast-paths.
func unmaskVectorized(b []byte, mask [4]byte) {
	// Construct 64-bit repeated mask
	mask32 := binary.LittleEndian.Uint32(mask[:])
	mask64 := uint64(mask32) | (uint64(mask32) << 32)

	i := 0
	for i+8 <= len(b) {
		w := binary.LittleEndian.Uint64(b[i:])
		binary.LittleEndian.PutUint64(b[i:], w^mask64)
		i += 8
	}

	for ; i < len(b); i++ {
		b[i] ^= mask[i%4]
	}
}

// WriteWSFrame encodes an RFC 6455 frame.
func WriteWSFrame(w io.Writer, opcode WSOpcode, fin bool, payload []byte, mask *[4]byte) error {
	var header [14]byte
	pos := 0

	b0 := byte(opcode)
	if fin {
		b0 |= 0x80
	}
	header[pos] = b0
	pos++

	length := len(payload)
	var b1 byte
	if mask != nil {
		b1 |= 0x80
	}

	if length <= 125 {
		header[pos] = b1 | byte(length)
		pos++
	} else if length <= 65535 {
		header[pos] = b1 | 126
		pos++
		binary.BigEndian.PutUint16(header[pos:], uint16(length))
		pos += 2
	} else {
		header[pos] = b1 | 127
		pos++
		binary.BigEndian.PutUint64(header[pos:], uint64(length))
		pos += 8
	}

	if mask != nil {
		copy(header[pos:], mask[:])
		pos += 4
	}

	if _, err := w.Write(header[:pos]); err != nil {
		return err
	}

	if length > 0 {
		if mask != nil {
			// Write masked copy
			maskedBuf := make([]byte, length)
			copy(maskedBuf, payload)
			unmaskVectorized(maskedBuf, *mask)
			if _, err := w.Write(maskedBuf); err != nil {
				return err
			}
		} else {
			if _, err := w.Write(payload); err != nil {
				return err
			}
		}
	}

	return nil
}
