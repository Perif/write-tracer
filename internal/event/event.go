package event

import (
	"bytes"
	"encoding/json"
	"strings"

	"write-tracer/internal/config"
)

type WriteEvent struct {
	Timestamp uint64                       `json:"timestamp"`
	Count     uint64                       `json:"count"`
	PID       uint32                       `json:"pid"`
	TID       uint32                       `json:"tid"`
	FD        uint32                       `json:"fd"`
	_         uint32                       // padding
	Comm      [config.MaxExecNameSize]byte `json:"comm"`
	Data      [config.MaxDataSize]byte     `json:"data"`
}

func (e WriteEvent) String() string {
	comm := string(bytes.TrimRight(e.Comm[:], "\x00"))
	data := strings.TrimRight(string(e.Data[:e.Count]), "\n\r")

	m := map[string]any{
		"timestamp": e.Timestamp,
		"pid":       e.PID,
		"tid":       e.TID,
		"comm":      comm,
		"fd":        e.FD,
		"count":     e.Count,
		"data":      data,
	}

	b, _ := json.Marshal(m)
	return string(b)
}

func (e WriteEvent) CommString() string {
	return string(bytes.TrimRight(e.Comm[:], "\x00"))
}

func (e WriteEvent) DataString() string {
	return strings.TrimRight(string(e.Data[:e.Count]), "\n\r")
}
