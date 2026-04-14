package net

import (
	"bytes"
	"encoding/binary"
)

// PP2_TYPE_SSL is the PROXY protocol v2 TLV type for SSL information.
// See https://www.haproxy.org/download/2.3/doc/proxy-protocol.txt section "TLV Format"
const PP2_TYPE_SSL = 0x20

// ParseProxyProtocolV2Header parses PROXY protocol v2 header to extract SSL information.
// See https://www.haproxy.org/download/2.3/doc/proxy-protocol.txt for the protocol specification.
// Returns true if the header indicates an SSL/TLS connection.
func ParseProxyProtocolV2Header(data []byte) bool {
	if len(data) < 16 {
		return false
	}

	sig := []byte{0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A}
	if !bytes.HasPrefix(data, sig) {
		return false
	}

	verCmd := data[12]
	version := (verCmd >> 4) & 0x0F
	if version != 2 {
		return false
	}

	len16 := binary.BigEndian.Uint16(data[14:16])
	tlvStart := 16
	tlvEnd := tlvStart + int(len16)

	if tlvEnd > len(data) {
		return false
	}

	pos := tlvStart
	for pos < tlvEnd {
		if pos+3 > tlvEnd {
			break
		}

		tlvType := data[pos]
		tlvLen := binary.BigEndian.Uint16(data[pos+1 : pos+3])

		if tlvType == PP2_TYPE_SSL {
			return true
		}

		pos += 3 + int(tlvLen)
	}

	return false
}
