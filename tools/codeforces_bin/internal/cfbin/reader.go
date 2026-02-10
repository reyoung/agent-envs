package cfbin

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

type CaseEntry struct {
	InputOffset  uint64
	InputLength  uint32
	OutputOffset uint64
	OutputLength uint32
}

type ProblemEntry struct {
	ContestID uint32
	Index     string
	Cases     []CaseEntry
}

type Reader struct {
	data   []byte
	header Header
	index  map[string]ProblemEntry
}

func NewReader(data []byte) (*Reader, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("data too small")
	}
	h, err := ReadHeader(bytes.NewReader(data[:HeaderSize]))
	if err != nil {
		return nil, err
	}
	endIndex := h.IndexOffset + h.IndexSize
	if endIndex > uint64(len(data)) {
		return nil, fmt.Errorf("index out of bounds")
	}
	index := make(map[string]ProblemEntry)
	idxReader := bytes.NewReader(data[h.IndexOffset:endIndex])
	for {
		var contestID uint32
		if err := binary.Read(idxReader, binary.LittleEndian, &contestID); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		var indexLen uint8
		if err := binary.Read(idxReader, binary.LittleEndian, &indexLen); err != nil {
			return nil, err
		}
		idxBytes := make([]byte, indexLen)
		if _, err := io.ReadFull(idxReader, idxBytes); err != nil {
			return nil, err
		}
		var caseCount uint32
		if err := binary.Read(idxReader, binary.LittleEndian, &caseCount); err != nil {
			return nil, err
		}
		cases := make([]CaseEntry, caseCount)
		for i := uint32(0); i < caseCount; i++ {
			var entry CaseEntry
			if err := binary.Read(idxReader, binary.LittleEndian, &entry.InputOffset); err != nil {
				return nil, err
			}
			if err := binary.Read(idxReader, binary.LittleEndian, &entry.InputLength); err != nil {
				return nil, err
			}
			if err := binary.Read(idxReader, binary.LittleEndian, &entry.OutputOffset); err != nil {
				return nil, err
			}
			if err := binary.Read(idxReader, binary.LittleEndian, &entry.OutputLength); err != nil {
				return nil, err
			}
			cases[i] = entry
		}
		key := makeKey(contestID, string(idxBytes))
		index[key] = ProblemEntry{
			ContestID: contestID,
			Index:     string(idxBytes),
			Cases:     cases,
		}
	}
	return &Reader{data: data, header: h, index: index}, nil
}

func (r *Reader) Get(contestID uint32, index string, caseNumber uint32) (input []byte, output []byte, total uint32, err error) {
	entry, ok := r.index[makeKey(contestID, index)]
	if !ok {
		return nil, nil, 0, fmt.Errorf("problem not found")
	}
	total = uint32(len(entry.Cases))
	if caseNumber >= total {
		return nil, nil, total, fmt.Errorf("case out of range")
	}
	caseEntry := entry.Cases[caseNumber]
	input, err = r.slice(caseEntry.InputOffset, caseEntry.InputLength)
	if err != nil {
		return nil, nil, total, err
	}
	output, err = r.slice(caseEntry.OutputOffset, caseEntry.OutputLength)
	if err != nil {
		return nil, nil, total, err
	}
	return input, output, total, nil
}

func (r *Reader) ListProblems() []ProblemEntry {
	out := make([]ProblemEntry, 0, len(r.index))
	for _, entry := range r.index {
		out = append(out, entry)
	}
	return out
}

func (r *Reader) slice(offset uint64, length uint32) ([]byte, error) {
	start := r.header.DataOffset + offset
	end := start + uint64(length)
	if end > uint64(len(r.data)) {
		return nil, fmt.Errorf("slice out of bounds")
	}
	return r.data[start:end], nil
}

func makeKey(contestID uint32, index string) string {
	return fmt.Sprintf("%d:%s", contestID, index)
}
