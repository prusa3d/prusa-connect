package webrtc

import (
	"io"
	"log/slog"
)

// accessUnitCeiling bounds one assembled access unit, the way unitCeiling
// bounds one NAL unit.
//
// The two are the same number because the reason is the same one. A 1080p
// keyframe from the cameras this product meets is under 200 KiB, so 4 MiB is
// two orders of margin over the largest legitimate picture; and unitCeiling
// alone bounds nothing here, because a device decides where an access unit
// ends. A device that sends legal units and never ends a picture would
// otherwise spend the host's memory a NAL unit at a time, per viewer.
const accessUnitCeiling = unitCeiling

// pictures groups the NAL units of an elementary stream into access units.
//
// One access unit is one coded picture, and one sample is one access unit. The
// grouping is what puts the RTP marker bit on the last packet of a frame rather
// than on the last packet of a NAL unit, which is how a decoder knows a picture
// is complete.
//
// Nothing before the first IDR is emitted. A viewer who joins in the middle of
// a group of pictures has no reference frame, so the alternative to waiting is
// a browser rendering artefacts until the next keyframe arrives.
type pictures struct {
	source *units

	// parameters is the most recent SPS and PPS, replayed in front of the
	// first emitted access unit. A camera that sends them once at the top of
	// the stream would otherwise give a viewer who joined later a keyframe no
	// decoder can configure itself from.
	sps []byte
	pps []byte

	pending  []byte
	hasSlice bool
	seenIDR  bool
	opened   bool

	// everyKey puts the parameter sets in front of every keyframe and not
	// only the first. A decoder in a browser that skipped to a keyframe after
	// falling behind then configures itself from that keyframe alone.
	everyKey bool

	// dropping is set when an access unit passed the ceiling. Every unit after
	// it is discarded until one arrives that can only begin an access unit,
	// which is how the assembler rejoins the stream at a boundary rather than
	// gluing the tail of a picture it threw away onto the front of the next
	// one. A decoder reads that splice as one picture with a syntax error.
	dropping bool

	// reported keeps the warning to one per stream. A device that overruns the
	// ceiling once overruns it every ceiling's worth of bytes after that, and a
	// line per overrun is the same memory attack against the log.
	reported bool

	log *slog.Logger

	// ceiling is what an access unit may not pass. It is a field so a test can
	// set a small one, as newUnitsWithLimit allows for one unit.
	ceiling int
}

func newPictures(source io.Reader) *pictures {
	return newPicturesWithCeiling(source, accessUnitCeiling)
}

// newPicturesWithCeiling is newPictures with the bound stated. The unit reader
// underneath takes the same number, because a bound on the picture that a
// single unit could pass on its own is not a bound.
func newPicturesWithCeiling(source io.Reader, ceiling int) *pictures {
	return &pictures{source: newUnitsWithLimit(source, ceiling), ceiling: ceiling}
}

// withLog gives the assembler somewhere to report a device that overran the
// access unit ceiling. It is separate from the constructor because a caller
// that has no logger is the common case in a test.
func (p *pictures) withLog(log *slog.Logger) *pictures {
	p.log = log

	return p
}

// next returns the next access unit, Annex-B framed with four-byte start codes.
// It reports io.EOF when the camera stream ends.
func (p *pictures) next() ([]byte, error) {
	for {
		unit, err := p.source.next()
		if err != nil {
			if err == io.EOF { //nolint:errorlint // the splitter returns io.EOF itself.
				if done := p.flush(); done != nil {
					return done, nil
				}
			}

			return nil, err
		}

		kind := unitType(unit)
		p.remember(kind, unit)

		if p.dropping {
			// The parameter sets above are still worth keeping, because they
			// are what configures the decoder for the picture this stream
			// resynchronises on.
			if !opensAccessUnit(kind, unit) {
				continue
			}

			p.dropping = false
		}

		if !p.starts(kind, unit) {
			p.append(unit)

			continue
		}

		done := p.flush()
		p.append(unit)

		if done != nil {
			return done, nil
		}
	}
}

// starts reports whether this unit begins a new access unit. H.264 section
// 7.4.1.2.4: a picture ends at the next parameter set, delimiter, or the first
// slice of the picture that follows.
//
// A picture is not one slice. x264 at its faster presets emits seven slices per
// picture, so a rule that started a new access unit at every slice would mark
// the end of a picture seven times for one frame and advance the RTP clock
// seven times as fast. first_mb_in_slice is what separates the two: it is zero
// on the first slice of a picture and nowhere else.
func (p *pictures) starts(kind uint8, unit []byte) bool {
	return p.hasSlice && opensAccessUnit(kind, unit)
}

// opensAccessUnit reports whether this unit can only be the beginning of an
// access unit, whatever stands in front of it.
//
// It is the boundary rule of starts without the "a picture is already being
// assembled" guard, so a stream that was dropped mid-picture rejoins at exactly
// the points the assembler would have cut at anyway. One rule and not two.
func opensAccessUnit(kind uint8, unit []byte) bool {
	switch kind {
	case unitSPS, unitPPS, unitSEI, unitAUD:
		return true
	default:
		return isPicture(kind) && firstSliceOfAPicture(unit)
	}
}

// firstSliceOfAPicture reads first_mb_in_slice, the leading ue(v) of every
// slice header, and reports whether it is zero.
//
// A unit too short to hold one is treated as a boundary. The alternative is
// gluing a malformed slice onto the picture in front of it, and a decoder reads
// that as one picture with a syntax error rather than as one bad slice.
func firstSliceOfAPicture(unit []byte) bool {
	const headerAndFirstBit = 2

	if len(unit) < headerAndFirstBit {
		return true
	}

	// The slice header starts at the bit after the one-byte NAL header, and
	// ue(v) is a run of zero bits, a one bit, then that many value bits. Zero
	// is the single bit 1, so the first bit decides it.
	return unit[1]&0x80 != 0
}

// remember keeps the parameter sets, which are replayed for a viewer who joined
// after they went by.
func (p *pictures) remember(kind uint8, unit []byte) {
	switch kind {
	case unitSPS:
		p.sps = append(p.sps[:0], unit...)
	case unitPPS:
		p.pps = append(p.pps[:0], unit...)
	}
}

// append adds one unit to the access unit being assembled, unless that unit is
// what carries the access unit past the ceiling.
//
// The keyframe is noted here rather than when the unit is read, because the
// access unit before it is flushed in between. Noting it earlier would emit the
// picture in front of the keyframe, which is the picture with no reference.
func (p *pictures) append(unit []byte) {
	if len(p.pending)+longStartCode+len(unit) > p.ceiling {
		p.discard(len(unit))

		return
	}

	kind := unitType(unit)

	if isPicture(kind) {
		p.hasSlice = true
	}

	if kind == unitIDR {
		p.seenIDR = true
	}

	p.pending = appendUnit(p.pending, unit)
}

// discard throws away the access unit being assembled and puts the assembler
// into resynchronisation.
//
// Dropping is chosen over ending the view because the two failures are not
// equal. A device that glitches for one picture costs a viewer a freeze until
// the next keyframe and then carries on; ending the session would turn that
// same glitch into a lost view and a reconnect, and a device that can be made
// to glitch on demand would then be a way to end anybody's view. Dropping is
// also the bound: a stream that never recovers holds nothing, because every
// unit after this one is discarded on arrival.
//
// Nothing partial is emitted. The bytes held are the front of a picture whose
// end never came, and half a picture handed to a decoder as a whole one is a
// picture with a syntax error. seenIDR goes with them, so nothing is emitted at
// all until the next keyframe: the pictures after a dropped one reference a
// picture this viewer never received.
func (p *pictures) discard(refused int) {
	if p.log != nil && !p.reported {
		p.reported = true
		p.log.Warn("a camera overran the access unit ceiling and its picture was dropped",
			"held", len(p.pending), "unit", refused, "ceiling", p.ceiling)
	}

	p.pending = nil
	p.hasSlice = false
	p.seenIDR = false
	p.dropping = true
}

// flush returns the assembled access unit, or nothing when there is nothing a
// decoder could use yet.
func (p *pictures) flush() []byte {
	assembled := p.pending
	held := p.hasSlice

	p.pending = nil
	p.hasSlice = false

	if !held || !p.seenIDR {
		return nil
	}

	if !p.opened || (p.everyKey && carries(assembled, unitIDR)) {
		p.opened = true
		assembled = p.lead(assembled)
	}

	return assembled
}

// lead puts the parameter sets in front of the first access unit a viewer sees,
// unless it already carries them.
func (p *pictures) lead(assembled []byte) []byte {
	if len(p.sps) == 0 && len(p.pps) == 0 {
		return assembled
	}

	if carries(assembled, unitSPS) {
		return assembled
	}

	lead := make([]byte, 0, len(p.sps)+len(p.pps)+len(assembled)+2*longStartCode)

	if len(p.sps) > 0 {
		lead = appendUnit(lead, p.sps)
	}

	if len(p.pps) > 0 {
		lead = appendUnit(lead, p.pps)
	}

	return append(lead, assembled...)
}

// carries reports whether an assembled access unit already holds a unit of this
// kind.
func carries(assembled []byte, kind uint8) bool {
	for at := 0; at < len(assembled); {
		found, width, ok := startCode(assembled, at)
		if !ok {
			return false
		}

		body := found + width
		if body < len(assembled) && unitType(assembled[body:]) == kind {
			return true
		}

		at = body
	}

	return false
}

// appendUnit writes one NAL unit with a four-byte start code. Four rather than
// three because it is what every payloader and every decoder accepts, and the
// three-byte form saves one byte per unit.
func appendUnit(dst []byte, unit []byte) []byte {
	dst = append(dst, 0, 0, 0, 1)

	return append(dst, unit...)
}
