package rtsp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prusa3d/prusa-connect/sdk/cameraapi"
)

const (
	// stillInterval is the shortest gap between two fetches from the device,
	// so many viewers do not become many device fetches.
	stillInterval = 10 * time.Second

	// stillCeiling bounds one still image.
	stillCeiling = 8 << 20
)

// ErrBadFrame reports a snapshot endpoint that did not answer with an image.
var ErrBadFrame = errors.New("rtsp: the camera did not answer with an image")

// stills fetches the still image of an RTSP camera over HTTP and keeps the
// last frame for every viewer. The host decides who may see it.
type stills struct {
	client    *http.Client
	endpoint  string
	authorize credential

	fetching sync.Mutex

	mu   sync.Mutex
	held cameraapi.Snapshot
}

func newStills(cfg Config, authorize credential) *stills {
	return &stills{
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: !cfg.Verify, //nolint:gosec // an operator opts in; see the option's summary.
					MinVersion:         tls.VersionTLS12,
				},
			},
		},
		endpoint:  cfg.SnapshotURL,
		authorize: authorize,
	}
}

// get returns the frame held for this camera, fetching a new one when the held
// one is older than the interval. A fetch that fails while a frame is held
// returns the held frame.
func (s *stills) get(ctx context.Context) (cameraapi.Snapshot, error) {
	if held, fresh := s.fresh(); fresh {
		return held, nil
	}

	s.fetching.Lock()
	defer s.fetching.Unlock()

	if held, fresh := s.fresh(); fresh {
		return held, nil
	}

	shot, err := s.fetch(ctx)
	if err != nil {
		if held, ok := s.last(); ok {
			return held, nil
		}

		return cameraapi.Snapshot{}, err
	}

	s.mu.Lock()
	s.held = shot
	s.mu.Unlock()

	return shot, nil
}

// close releases the idle connections to the device.
func (s *stills) close() { s.client.CloseIdleConnections() }

func (s *stills) fresh() (cameraapi.Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.held.Taken.IsZero() || time.Since(s.held.Taken) >= stillInterval {
		return cameraapi.Snapshot{}, false
	}

	return s.held, true
}

func (s *stills) last() (cameraapi.Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.held.Taken.IsZero() {
		return cameraapi.Snapshot{}, false
	}

	return s.held, true
}

// fetch reads one image from the device.
func (s *stills) fetch(ctx context.Context) (cameraapi.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint, nil)
	if err != nil {
		return cameraapi.Snapshot{}, fmt.Errorf("build a snapshot request: %w", scrub(err))
	}

	if s.authorize != nil {
		// The HTTP endpoint is sent the RTSP credential as Basic, without
		// waiting for a challenge.
		if header := s.authorize(http.MethodGet, s.endpoint, "Basic", 1); header != "" {
			req.Header.Set("Authorization", header)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return cameraapi.Snapshot{}, fmt.Errorf("fetch a snapshot: %w", scrub(err))
	}

	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNoContent:
		return cameraapi.Snapshot{}, cameraapi.ErrNoFrame
	default:
		return cameraapi.Snapshot{}, fmt.Errorf("%w: the snapshot endpoint answered %d",
			ErrBadFrame, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, stillCeiling+1))
	if err != nil {
		return cameraapi.Snapshot{}, fmt.Errorf("read a camera frame: %w", scrub(err))
	}

	if len(body) > stillCeiling {
		return cameraapi.Snapshot{}, fmt.Errorf("%w: the frame is over the %d byte ceiling",
			ErrBadFrame, stillCeiling)
	}

	if !isJPEG(body) {
		return cameraapi.Snapshot{}, fmt.Errorf("%w: the body does not start with a JPEG marker", ErrBadFrame)
	}

	return cameraapi.Snapshot{
		Bytes:     body,
		MediaType: cameraapi.FormatJPEG.MediaType(),
		Taken:     takenAt(resp),
	}, nil
}

// isJPEG reports whether the bytes start with the JPEG start-of-image marker.
// A camera is untrusted, and its answer is served to a browser on this origin.
func isJPEG(body []byte) bool {
	return len(body) >= 3 && bytes.Equal(body[:3], []byte{0xFF, 0xD8, 0xFF})
}

// takenAt is the device's Last-Modified time, or the time of the read.
func takenAt(resp *http.Response) time.Time {
	if stamp := resp.Header.Get("Last-Modified"); stamp != "" {
		if parsed, err := http.ParseTime(stamp); err == nil {
			return parsed.UTC()
		}
	}

	return time.Now().UTC()
}

// scrub removes the URL from a transport error. net/http puts the whole URL in
// its errors, and a camera URL may contain a credential.
func scrub(err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return fmt.Errorf("%s the camera: %w", urlErr.Op, urlErr.Err)
	}

	return err
}
