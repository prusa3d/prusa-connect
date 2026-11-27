package printerpairing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// codeAlphabet is the characters a pairing code is drawn from. Buddy
	// firmware accepts Crockford's base32, 0 to 9 and A to Z without I, L, O
	// and U. This alphabet also drops 0 and 1, so a person reading the code
	// off the printer cannot take either for a letter.
	codeAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

	// codeHalf is the length of each half of XXXX-XXXX.
	codeHalf = 4

	// byteSpace is the number of values one random byte takes, which
	// rejection sampling divides to keep every character equally likely.
	byteSpace = 256

	idBytes = 16
)

// NewCode draws a pairing code in the shape the firmware takes, XXXX-XXXX,
// from the operating system's random source.
func NewCode() (string, error) {
	limit := byte(byteSpace - byteSpace%len(codeAlphabet))

	var out strings.Builder

	out.Grow(2*codeHalf + 1)

	buf := make([]byte, 1)

	for drawn := 0; drawn < 2*codeHalf; {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("draw a pairing code: %w", err)
		}

		if buf[0] >= limit {
			continue
		}

		if drawn == codeHalf {
			out.WriteByte('-')
		}

		out.WriteByte(codeAlphabet[int(buf[0])%len(codeAlphabet)])

		drawn++
	}

	return out.String(), nil
}

// newID is the id a pairing is addressed by on this server's API. It is not
// the code: the code is on the printer's screen for anybody standing there to
// read, and the id is known only to the person who started the pairing.
func newID() (string, error) {
	buf := make([]byte, idBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("draw a pairing id: %w", err)
	}

	return hex.EncodeToString(buf), nil
}
