package qlab

import (
	"encoding/binary"
	"fmt"
	"math"
)

func oscPad(n int) int {
	return (4 - n%4) % 4
}

func buildOSC(addr string, args ...any) []byte {
	var buf []byte

	buf = append(buf, []byte(addr)...)
	buf = append(buf, 0)
	for range oscPad(len(addr) + 1) {
		buf = append(buf, 0)
	}

	typetag := ","
	for _, arg := range args {
		switch arg.(type) {
		case int32:
			typetag += "i"
		case float32:
			typetag += "f"
		case string:
			typetag += "s"
		case []byte:
			typetag += "b"
		case int64:
			typetag += "h"
		case float64:
			typetag += "d"
		}
	}
	buf = append(buf, []byte(typetag)...)
	buf = append(buf, 0)
	for range oscPad(len(typetag) + 1) {
		buf = append(buf, 0)
	}

	for _, arg := range args {
		switch v := arg.(type) {
		case int32:
			buf = binary.BigEndian.AppendUint32(buf, uint32(v))
		case float32:
			buf = binary.BigEndian.AppendUint32(buf, math.Float32bits(v))
		case string:
			buf = append(buf, []byte(v)...)
			buf = append(buf, 0)
			for range oscPad(len(v) + 1) {
				buf = append(buf, 0)
			}
		case []byte:
			buf = binary.BigEndian.AppendUint32(buf, uint32(len(v)))
			buf = append(buf, v...)
			for range oscPad(len(v)) {
				buf = append(buf, 0)
			}
		case int64:
			buf = binary.BigEndian.AppendUint64(buf, uint64(v))
		case float64:
			buf = binary.BigEndian.AppendUint64(buf, math.Float64bits(v))
		}
	}

	return buf
}

func parseOSC(data []byte) (addr string, args []any, err error) {
	if len(data) < 4 {
		return "", nil, fmt.Errorf("osc: message too short")
	}

	end := 0
	for end < len(data) && data[end] != 0 {
		end++
	}
	addr = string(data[:end])
	pos := end + 1 + oscPad(end+1)

	if pos >= len(data) || data[pos] != ',' {
		return addr, nil, nil
	}

	ttEnd := pos
	for ttEnd < len(data) && data[ttEnd] != 0 {
		ttEnd++
	}
	typetag := string(data[pos+1 : ttEnd])
	pos = ttEnd + 1 + oscPad(ttEnd-pos+1)

	for _, t := range typetag {
		switch t {
		case 'i':
			if pos+4 > len(data) {
				return addr, args, fmt.Errorf("osc: truncated int32")
			}
			args = append(args, int32(binary.BigEndian.Uint32(data[pos:])))
			pos += 4
		case 'f':
			if pos+4 > len(data) {
				return addr, args, fmt.Errorf("osc: truncated float32")
			}
			args = append(args, math.Float32frombits(binary.BigEndian.Uint32(data[pos:])))
			pos += 4
		case 's':
			end := pos
			for end < len(data) && data[end] != 0 {
				end++
			}
			args = append(args, string(data[pos:end]))
			pos = end + 1 + oscPad(end-pos+1)
		case 'b':
			if pos+4 > len(data) {
				return addr, args, fmt.Errorf("osc: truncated blob size")
			}
			size := int(binary.BigEndian.Uint32(data[pos:]))
			pos += 4
			if pos+size > len(data) {
				return addr, args, fmt.Errorf("osc: truncated blob")
			}
			b := make([]byte, size)
			copy(b, data[pos:pos+size])
			args = append(args, b)
			pos += size + oscPad(size)
		case 'h':
			if pos+8 > len(data) {
				return addr, args, fmt.Errorf("osc: truncated int64")
			}
			args = append(args, int64(binary.BigEndian.Uint64(data[pos:])))
			pos += 8
		case 'd':
			if pos+8 > len(data) {
				return addr, args, fmt.Errorf("osc: truncated float64")
			}
			args = append(args, math.Float64frombits(binary.BigEndian.Uint64(data[pos:])))
			pos += 8
		case 'T':
			args = append(args, true)
		case 'F':
			args = append(args, false)
		case 'N':
			args = append(args, nil)
		}
	}

	return addr, args, nil
}
