package rtsp

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// The RTP payload types of RFC 6184 for H.264. Types 1 to 23 are one whole NAL
// unit. 24 aggregates several units and 28 fragments one.
const (
	payloadSTAPA uint8 = 24
	payloadFUA   uint8 = 28
)

// The RTP header, RFC 3550 section 5.1. The fixed part is twelve bytes.
const (
	rtpHeader     = 12
	rtpExtension  = 0x10
	rtpPadding    = 0x20
	rtpCSRCMask   = 0x0F
	rtpMarkerMask = 0x80

	// The FU-A header flags, RFC 6184 section 5.8: the first fragment of a unit
	// and the last one.
	fuStart = 0x80
	fuEnd   = 0x40

	// csrcWidth is the size of one contributing source identifier and of one
	// extension word.
	csrcWidth = 4
)

// startCode is the Annex-B delimiter written before every unit.
//
//nolint:gochecknoglobals // a byte slice constant; Go has no other spelling.
var startCode = []byte{0, 0, 0, 1}

// readSDP finds the H.264 video track in a DESCRIBE answer. A track with any
// other codec is refused here, before the first frame.
func readSDP(body []byte) (describe, error) {
	var (
		found   describe
		media   bool
		payload int64 = -1
	)

	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimRight(line, "\r")

		switch {
		case strings.HasPrefix(line, "m="):
			if media {
				// A second media section, so the video track is complete.
				return complete(found, payload)
			}

			media = strings.HasPrefix(line, "m=video")
			payload = firstFormat(line)
		case !media:
			continue
		case strings.HasPrefix(line, "a=control:"):
			found.control = strings.TrimPrefix(line, "a=control:")
		case strings.HasPrefix(line, "a=rtpmap:"):
			if kind, at := rtpmap(line); at == payload && !strings.EqualFold(kind, "H264") {
				return describe{}, fmt.Errorf("%w: it describes %s", ErrNoH264, kind)
			}
		case strings.HasPrefix(line, "a=fmtp:"):
			found.parameters = append(found.parameters, sprop(line)...)
		}
	}

	return complete(found, payload)
}

// complete refuses a description with no usable video track.
func complete(found describe, payload int64) (describe, error) {
	if payload < 0 {
		return describe{}, fmt.Errorf("%w: it describes no video at all", ErrNoH264)
	}

	found.payload = uint8(payload) //nolint:gosec // firstFormat bounds it to 0..127.

	return found, nil
}

// firstFormat reads the first payload type off an m= line.
func firstFormat(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 4 || !strings.HasPrefix(line, "m=video") {
		return -1
	}

	at, err := strconv.ParseInt(fields[3], 10, 16)
	if err != nil || at < 0 || at > 127 {
		return -1
	}

	return at
}

// rtpmap reads the encoding name and the payload type it belongs to.
func rtpmap(line string) (string, int64) {
	rest := strings.TrimPrefix(line, "a=rtpmap:")

	number, encoding, found := strings.Cut(rest, " ")
	if !found {
		return "", -1
	}

	at, err := strconv.ParseInt(strings.TrimSpace(number), 10, 16)
	if err != nil {
		return "", -1
	}

	name, _, _ := strings.Cut(encoding, "/")

	return strings.TrimSpace(name), at
}

// sprop decodes the parameter sets of an fmtp line: the SPS and PPS a decoder
// needs before a keyframe.
func sprop(line string) [][]byte {
	_, rest, found := strings.Cut(line, " ")
	if !found {
		return nil
	}

	var sets [][]byte

	for pair := range strings.SplitSeq(rest, ";") {
		key, value, split := strings.Cut(strings.TrimSpace(pair), "=")
		if !split || strings.ToLower(key) != "sprop-parameter-sets" {
			continue
		}

		for encoded := range strings.SplitSeq(value, ",") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
			if err != nil || len(decoded) == 0 {
				continue
			}

			sets = append(sets, decoded)
		}
	}

	return sets
}

// depacketizer turns RTP payloads into an Annex-B elementary stream. It uses
// only the standard library, so a driver adds no media dependency.
type depacketizer struct {
	unit  []byte
	limit int
	inFU  bool
	lost  bool
}

func newDepacketizer(limit int) *depacketizer { return &depacketizer{limit: limit} }

// drop abandons the unit being reassembled, so the fragments that follow a
// malformed packet are not joined to the ones before it.
func (d *depacketizer) drop() {
	d.inFU = false
	d.unit = d.unit[:0]
}

// append reads one RTP packet and appends the NAL units it completed to out,
// Annex-B framed. A fragment that completes no unit appends nothing.
func (d *depacketizer) append(out []byte, packet []byte) ([]byte, error) {
	payload, err := rtpPayload(packet)
	if err != nil {
		return out, err
	}

	if len(payload) == 0 {
		return out, nil
	}

	switch payload[0] & 0x1F {
	case payloadSTAPA:
		return d.aggregated(out, payload)
	case payloadFUA:
		return d.fragment(out, payload)
	default:
		d.inFU = false

		return appendUnit(out, payload), nil
	}
}

// aggregated unpacks a STAP-A: several small units in one packet.
func (d *depacketizer) aggregated(out []byte, payload []byte) ([]byte, error) {
	d.inFU = false

	for at := 1; at+2 <= len(payload); {
		size := int(binary.BigEndian.Uint16(payload[at:]))
		at += 2

		if size == 0 || at+size > len(payload) {
			return out, fmt.Errorf("%w: an aggregated unit runs past the packet", ErrHandshake)
		}

		out = appendUnit(out, payload[at:at+size])
		at += size
	}

	return out, nil
}

// fragment reassembles a FU-A: one unit spread over several packets. A
// fragment without its start is dropped, because a decoder would misread half
// a unit.
func (d *depacketizer) fragment(out []byte, payload []byte) ([]byte, error) {
	const fuaHeader = 2

	if len(payload) <= fuaHeader {
		return out, nil
	}

	start := payload[1]&fuStart != 0
	end := payload[1]&fuEnd != 0

	switch {
	case start:
		d.unit = append(d.unit[:0], payload[0]&0xE0|payload[1]&0x1F)
		d.inFU = true
		d.lost = false
	case !d.inFU:
		// The start of this unit was never seen.
		return out, nil
	}

	if len(d.unit)+len(payload)-fuaHeader > d.limit {
		d.inFU = false
		d.lost = true
		d.unit = d.unit[:0]

		return out, fmt.Errorf("%w: a fragmented unit passed %d bytes", ErrHandshake, d.limit)
	}

	d.unit = append(d.unit, payload[fuaHeader:]...)

	if !end {
		return out, nil
	}

	d.inFU = false

	if d.lost {
		return out, nil
	}

	return appendUnit(out, d.unit), nil
}

// rtpPayload strips the RTP header, its contributing sources, its extension
// and its padding.
func rtpPayload(packet []byte) ([]byte, error) {
	if len(packet) < rtpHeader {
		return nil, fmt.Errorf("%w: an RTP packet is %d bytes", ErrHandshake, len(packet))
	}

	at := rtpHeader + int(packet[0]&rtpCSRCMask)*csrcWidth
	if at > len(packet) {
		return nil, fmt.Errorf("%w: the contributing source list runs past the packet", ErrHandshake)
	}

	if packet[0]&rtpExtension != 0 {
		if at+csrcWidth > len(packet) {
			return nil, fmt.Errorf("%w: the extension header runs past the packet", ErrHandshake)
		}

		at += csrcWidth + int(binary.BigEndian.Uint16(packet[at+2:]))*csrcWidth
		if at > len(packet) {
			return nil, fmt.Errorf("%w: the extension runs past the packet", ErrHandshake)
		}
	}

	end := len(packet)

	if packet[0]&rtpPadding != 0 {
		pad := int(packet[end-1])
		if pad == 0 || end-pad < at {
			return nil, fmt.Errorf("%w: the padding runs past the packet", ErrHandshake)
		}

		end -= pad
	}

	return packet[at:end], nil
}

// marked reports whether the packet has the RTP marker bit set. For H.264 it
// marks the last packet of an access unit.
//
//nolint:unparam // no caller acts on the boundary yet; the read in rtsp.go keeps the bit decoded in one place.
func marked(packet []byte) bool {
	return len(packet) > 1 && packet[1]&rtpMarkerMask != 0
}

// appendUnit writes one NAL unit with its Annex-B start code.
func appendUnit(out []byte, unit []byte) []byte {
	out = append(out, startCode...)

	return append(out, unit...)
}
