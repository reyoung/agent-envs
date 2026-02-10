package cfbin

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	Magic      = "CFPDBIN1"
	Version    = 1
	HeaderSize = 48
)

var (
	ErrBadMagic   = errors.New("invalid magic")
	ErrBadVersion = errors.New("unsupported version")
)

type Header struct {
	Magic       [8]byte
	Version     uint32
	Reserved    uint32
	DataOffset  uint64
	DataSize    uint64
	IndexOffset uint64
	IndexSize   uint64
}

func NewHeader(dataSize, indexSize uint64) Header {
	var h Header
	copy(h.Magic[:], []byte(Magic))
	h.Version = Version
	h.DataOffset = HeaderSize
	h.DataSize = dataSize
	h.IndexOffset = HeaderSize + dataSize
	h.IndexSize = indexSize
	return h
}

func ReadHeader(r io.Reader) (Header, error) {
	var h Header
	if err := binary.Read(r, binary.LittleEndian, &h); err != nil {
		return h, err
	}
	if string(h.Magic[:]) != Magic {
		return h, ErrBadMagic
	}
	if h.Version != Version {
		return h, ErrBadVersion
	}
	if h.DataOffset != HeaderSize {
		return h, fmt.Errorf("unexpected data offset %d", h.DataOffset)
	}
	return h, nil
}

func WriteHeader(w io.Writer, h Header) error {
	return binary.Write(w, binary.LittleEndian, &h)
}
