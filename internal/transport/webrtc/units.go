package webrtc

import (
	"io"
	"log/slog"
)

// AccessUnits is a camera's H.264 elementary stream read as whole pictures,
// for a viewer that is not a peer connection: the live view a browser falls
// back to when it cannot reach this server over WebRTC.
//
// It is in this package because the framing is this package's. The same
// assembler cuts the stream into pictures for both views, so a camera that
// plays over one plays over the other.
type AccessUnits struct {
	frames   io.ReadCloser
	pictures *pictures
}

// NewAccessUnits reads frames as access units. It takes ownership of frames,
// which Close closes.
func NewAccessUnits(frames io.ReadCloser, log *slog.Logger) *AccessUnits {
	pictures := newPictures(frames).withLog(log)
	pictures.everyKey = true

	return &AccessUnits{frames: frames, pictures: pictures}
}

// Next returns the next access unit, Annex-B framed with four-byte start
// codes, and whether it is a keyframe. A keyframe carries the parameter sets a
// decoder configures itself from. The first unit is always a keyframe. It
// reports io.EOF when the camera stream ends.
func (a *AccessUnits) Next() ([]byte, bool, error) {
	unit, err := a.pictures.next()
	if err != nil {
		return nil, false, err //nolint:wrapcheck // io.EOF is compared by identity.
	}

	return unit, carries(unit, unitIDR), nil
}

// Close closes the camera stream, which gives the camera's stream slot back.
func (a *AccessUnits) Close() error {
	return a.frames.Close() //nolint:wrapcheck // the camera's own close, reported as it is.
}
