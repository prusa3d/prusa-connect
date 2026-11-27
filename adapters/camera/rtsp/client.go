package rtsp

import (
	"bufio"
	"context"
	"crypto/md5" //nolint:gosec // RFC 7616 fixes MD5 for digest access authentication.
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The RTSP methods this client sends. It only plays.
const (
	methodOptions  = "OPTIONS"
	methodDescribe = "DESCRIBE"
	methodSetup    = "SETUP"
	methodPlay     = "PLAY"
	methodTeardown = "TEARDOWN"
)

const (
	// defaultPort is the RTSP port, fixed by RFC 2326 section 3.2.
	defaultPort = "554"

	// interleavedHeader is the size of the frame header of RTP inside the RTSP
	// connection: a dollar sign, a channel, and a 16-bit length.
	interleavedHeader = 4

	// packetCeiling is the largest interleaved packet. The length field is
	// 16 bits.
	packetCeiling = 1 << 16

	// headerCeiling bounds one line of an RTSP response's head.
	headerCeiling = 8 << 10

	// bodyCeiling bounds a response body, which is an SDP document.
	bodyCeiling = 64 << 10

	// keepaliveShare refreshes the session at half its declared timeout, so
	// one lost message does not end it.
	keepaliveShare = 2

	// readWindowShare is how many timeouts a read may block. A camera at a
	// slow frame rate must not look like a dead connection.
	readWindowShare = 2

	// resyncCeiling is how many stray bytes next skips looking for the next
	// packet before it gives the stream up. It is one packet's worth: a
	// camera that loses its framing for longer is not sending video.
	resyncCeiling = packetCeiling

	// headerCount is the most headers one response may have.
	headerCount = 64

	// digestFieldCount, addedHeaders and expectedHeaders are map size hints.
	digestFieldCount = 6

	addedHeaders = 2

	expectedHeaders = 8
)

var (
	// ErrHandshake reports a device answer this client cannot act on. It
	// never contains the URL, which may include a credential.
	ErrHandshake = errors.New("rtsp: the camera did not complete the handshake")
	// ErrUnauthorized reports a device that asked for credentials this camera
	// was not configured with, or refused the ones it was.
	ErrUnauthorized = errors.New("rtsp: the camera refused the credentials")
	// ErrNoH264 reports a session that describes no H.264 video track.
	ErrNoH264 = errors.New("rtsp: the camera describes no H.264 video track")
)

// credential answers a device's challenge. It is a closure so that printing a
// struct that has one does not print the password.
type credential func(method, uri, challenge string) string

// session is one playing RTSP connection. Two goroutines write to it, one for
// the keepalive and one for the teardown, so writing guards the socket and seq.
type session struct {
	writing sync.Mutex
	conn    net.Conn
	reader  *bufio.Reader
	base    string
	id      string
	channel byte
	seq     int
	timeout time.Duration
	auth    credential
	control string

	// parameters is the SPS and PPS the SDP declared. Some cameras send them
	// only there.
	parameters [][]byte
}

// describe is what the SDP says about the video track.
type describe struct {
	control    string
	payload    uint8
	parameters [][]byte
}

// dial opens a connection, negotiates one H.264 video track and starts it
// playing. Media is interleaved on the same TCP connection. UDP transport is
// not offered.
func dial(
	ctx context.Context, target string, auth credential, timeout time.Duration, secure bool,
) (*session, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("%w: the URL could not be parsed", ErrHandshake)
	}

	conn, err := connect(ctx, parsed, timeout, secure)
	if err != nil {
		return nil, err
	}

	live := &session{
		conn:    conn,
		reader:  bufio.NewReaderSize(conn, packetCeiling),
		base:    stripped(parsed),
		timeout: timeout,
		auth:    auth,
	}

	if err = live.play(); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return live, nil
}

// connect opens the transport. rtsps is RTSP over TLS.
func connect(
	ctx context.Context, parsed *url.URL, timeout time.Duration, secure bool,
) (net.Conn, error) {
	host := parsed.Host
	if parsed.Port() == "" {
		host = net.JoinHostPort(parsed.Hostname(), defaultPort)
	}

	dialer := &net.Dialer{Timeout: timeout}

	conn, err := dialer.Dial("tcp", host)
	if err != nil {
		return nil, fmt.Errorf("%w: %s the camera", ErrHandshake, opOf(err))
	}

	if parsed.Scheme != "rtsps" {
		return conn, nil
	}

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         parsed.Hostname(),
		InsecureSkipVerify: !secure, //nolint:gosec // an operator opts in; a camera on a LAN rarely has a verifiable certificate.
		MinVersion:         tls.VersionTLS12,
	})

	handshake, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err = tlsConn.HandshakeContext(handshake); err != nil {
		_ = conn.Close()

		return nil, fmt.Errorf("%w: the TLS handshake failed", ErrHandshake)
	}

	return tlsConn, nil
}

// play runs OPTIONS, DESCRIBE, SETUP and PLAY.
func (s *session) play() error {
	if _, _, err := s.request(methodOptions, s.base, nil); err != nil {
		return err
	}

	head, body, err := s.request(methodDescribe, s.base,
		map[string]string{"Accept": "application/sdp"})
	if err != nil {
		return err
	}

	if base := head["content-base"]; base != "" {
		s.base = strings.TrimSuffix(base, "/")
	}

	track, err := readSDP(body)
	if err != nil {
		return err
	}

	s.control = absolute(s.base, track.control)
	s.parameters = track.parameters

	setup, _, err := s.request(methodSetup, s.control, map[string]string{
		"Transport": "RTP/AVP/TCP;unicast;interleaved=0-1",
	})
	if err != nil {
		return err
	}

	s.id, s.timeout = readSession(setup["session"], s.timeout)
	s.channel = readChannel(setup["transport"])

	if _, _, err = s.request(methodPlay, s.base, map[string]string{"Range": "npt=0.000-"}); err != nil {
		return err
	}

	return nil
}

// next returns the next interleaved RTP packet on the media channel. An RTSP
// response on the same connection, such as a keepalive answer, is read and
// discarded. Bytes that are neither are skipped up to the next packet. A camera
// whose server names itself rtsp_demo sends such bytes every few seconds, and
// ffmpeg plays it by skipping them the same way.
func (s *session) next() ([]byte, error) {
	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(s.readWindow())); err != nil {
			return nil, fmt.Errorf("%w: the connection would not take a deadline", ErrHandshake)
		}

		lead, err := s.reader.ReadByte()
		if err != nil {
			return nil, streamError(err)
		}

		if lead != '$' {
			if err = s.reader.UnreadByte(); err != nil {
				return nil, streamError(err)
			}

			if err = s.skipStray(); err != nil {
				return nil, err
			}

			continue
		}

		header := make([]byte, interleavedHeader-1)
		if _, err = io.ReadFull(s.reader, header); err != nil {
			return nil, streamError(err)
		}

		packet := make([]byte, binary.BigEndian.Uint16(header[1:]))
		if _, err = io.ReadFull(s.reader, packet); err != nil {
			return nil, streamError(err)
		}

		if header[0] != s.channel {
			// The odd channel is RTCP, which this client does not act on.
			continue
		}

		return packet, nil
	}
}

// skipStray reads what stands where a packet was expected. An RTSP response is
// read and discarded. Anything else is skipped up to a dollar sign that opens
// a packet on this session's media or report channel.
func (s *session) skipStray() error {
	if head, err := s.reader.Peek(len("RTSP/")); err == nil && string(head) == "RTSP/" {
		_, _, err = s.readResponse()

		return err
	}

	// A packet opens with a dollar sign and its channel.
	const opening = 2

	for range resyncCeiling {
		head, err := s.reader.Peek(opening)
		if err != nil {
			return streamError(err)
		}

		if head[0] == '$' && (head[1] == s.channel || head[1] == s.channel+1) {
			return nil
		}

		if _, err = s.reader.Discard(1); err != nil {
			return streamError(err)
		}
	}

	return fmt.Errorf("%w: %d bytes arrived that are neither a packet nor a response",
		ErrHandshake, resyncCeiling)
}

// keepalive tells the device the session is still wanted. It does not read the
// answer. The answer arrives between media packets on the same connection, and
// next discards it there.
func (s *session) keepalive() error {
	if err := s.conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
		return fmt.Errorf("%w: the connection would not take a deadline", ErrHandshake)
	}

	return s.write(methodOptions, s.base, map[string]string{"Session": s.id})
}

// interval is how often keepalive is due.
func (s *session) interval() time.Duration { return s.timeout / keepaliveShare }

// readWindow is how long a read may block before the connection counts as dead.
func (s *session) readWindow() time.Duration {
	return max(s.timeout*readWindowShare, time.Minute)
}

// close sends TEARDOWN, ignoring its failure, and closes the connection.
func (s *session) close() error {
	if s.id != "" {
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.timeout))
		_ = s.write(methodTeardown, s.base, map[string]string{"Session": s.id})
	}

	return s.conn.Close()
}

// request sends one method and reads its answer, retrying once with credentials
// when the device challenges.
func (s *session) request(method, uri string, headers map[string]string) (map[string]string, []byte, error) {
	head, body, err := s.send(method, uri, headers, "")
	if err != nil {
		return nil, nil, err
	}

	status := head[":status"]
	if status == "401" {
		challenge := head["www-authenticate"]
		if s.auth == nil || challenge == "" {
			return nil, nil, fmt.Errorf("%w: it asked for credentials", ErrUnauthorized)
		}

		head, body, err = s.send(method, uri, headers, s.auth(method, uri, challenge))
		if err != nil {
			return nil, nil, err
		}

		status = head[":status"]
	}

	switch status {
	case "200":
		return head, body, nil
	case "401":
		return nil, nil, fmt.Errorf("%w: it refused them", ErrUnauthorized)
	default:
		return nil, nil, fmt.Errorf("%w: %s answered %s", ErrHandshake, method, status)
	}
}

// send writes one message and reads one response.
func (s *session) send(
	method, uri string, headers map[string]string, authorization string,
) (map[string]string, []byte, error) {
	if err := s.conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
		return nil, nil, fmt.Errorf("%w: the connection would not take a deadline", ErrHandshake)
	}

	all := make(map[string]string, len(headers)+addedHeaders)
	maps.Copy(all, headers)

	if s.id != "" {
		all["Session"] = s.id
	}

	if authorization != "" {
		all["Authorization"] = authorization
	}

	if err := s.write(method, uri, all); err != nil {
		return nil, nil, err
	}

	if err := s.conn.SetReadDeadline(time.Now().Add(s.timeout)); err != nil {
		return nil, nil, fmt.Errorf("%w: the connection would not take a deadline", ErrHandshake)
	}

	return s.readResponse()
}

// write sends one RTSP message.
func (s *session) write(method, uri string, headers map[string]string) error {
	s.writing.Lock()
	defer s.writing.Unlock()

	s.seq++

	var out strings.Builder

	out.WriteString(method + " " + uri + " RTSP/1.0\r\n")
	out.WriteString("CSeq: " + strconv.Itoa(s.seq) + "\r\n")
	out.WriteString("User-Agent: prusa-connect\r\n")

	for key, value := range headers {
		out.WriteString(key + ": " + value + "\r\n")
	}

	out.WriteString("\r\n")

	if _, err := io.WriteString(s.conn, out.String()); err != nil {
		return fmt.Errorf("%w: %s the camera", ErrHandshake, opOf(err))
	}

	return nil
}

// skipInterleaved drops the interleaved packets in front of the next RTSP
// answer. A camera may start the media before it answers PLAY: a Buddy3D's
// LIVE555 server sends an RTCP report and the first RTP packets ahead of the
// answer. They are dropped rather than read as the answer, and the stream
// starts at the next keyframe, with the parameter sets the SDP carried.
func (s *session) skipInterleaved() error {
	for {
		first, err := s.reader.Peek(1)
		if err != nil {
			return streamError(err)
		}

		if first[0] != '$' {
			return nil
		}

		header := make([]byte, interleavedHeader)
		if _, err = io.ReadFull(s.reader, header); err != nil {
			return streamError(err)
		}

		if _, err = s.reader.Discard(int(binary.BigEndian.Uint16(header[2:]))); err != nil {
			return streamError(err)
		}
	}
}

// readResponse reads one RTSP response, returning its headers under lowercase
// keys with the status line under ":status".
func (s *session) readResponse() (map[string]string, []byte, error) {
	head := make(map[string]string, expectedHeaders)

	if err := s.skipInterleaved(); err != nil {
		return nil, nil, err
	}

	line, err := s.line()
	if err != nil {
		return nil, nil, err
	}

	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "RTSP/") {
		return nil, nil, fmt.Errorf("%w: the answer is not an RTSP response", ErrHandshake)
	}

	head[":status"] = fields[1]

	for read := 0; ; read++ {
		if read > headerCount {
			return nil, nil, fmt.Errorf("%w: the response carries too many headers", ErrHandshake)
		}

		line, err = s.line()
		if err != nil {
			return nil, nil, err
		}

		if line == "" {
			break
		}

		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		head[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}

	length, _ := strconv.Atoi(head["content-length"])
	if length <= 0 {
		return head, nil, nil
	}

	if length > bodyCeiling {
		return nil, nil, fmt.Errorf("%w: the body is %d bytes", ErrHandshake, length)
	}

	body := make([]byte, length)
	if _, err = io.ReadFull(s.reader, body); err != nil {
		return nil, nil, streamError(err)
	}

	return head, body, nil
}

// line reads one header line and rejects one over headerCeiling.
func (s *session) line() (string, error) {
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", streamError(err)
	}

	if len(line) > headerCeiling {
		return "", fmt.Errorf("%w: a header line is %d bytes", ErrHandshake, len(line))
	}

	return strings.TrimRight(line, "\r\n"), nil
}

// sprop returns the SDP's sprop-parameter-sets, so a viewer who joins between
// two keyframes still gets an SPS and a PPS.
func (s *session) sprop() [][]byte { return s.parameters }

// basicAuth answers a Basic challenge.
func basicAuth(username, password string) credential {
	encoded := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))

	return func(_, _, challenge string) string {
		if !strings.HasPrefix(strings.ToLower(challenge), "basic") {
			return ""
		}

		return "Basic " + encoded
	}
}

// digestOrBasic answers whichever scheme the device asked for. Basic sends the
// password in the clear unless the URL is rtsps.
func digestOrBasic(username, password string) credential {
	basic := basicAuth(username, password)

	return func(method, uri, challenge string) string {
		if !strings.HasPrefix(strings.ToLower(challenge), "digest") {
			return basic(method, uri, challenge)
		}

		fields := digestFields(challenge)

		realm, nonce := fields["realm"], fields["nonce"]
		if realm == "" || nonce == "" {
			return ""
		}

		ha1 := sum(username + ":" + realm + ":" + password)
		ha2 := sum(method + ":" + uri)

		out := `Digest username="` + username + `", realm="` + realm +
			`", nonce="` + nonce + `", uri="` + uri + `"`

		if opaque := fields["opaque"]; opaque != "" {
			out += `, opaque="` + opaque + `"`
		}

		if qop := qopOf(fields["qop"]); qop != "" {
			cnonce := nonceValue()
			out += `, qop=` + qop + `, nc=00000001, cnonce="` + cnonce + `"`

			return out + `, response="` +
				sum(ha1+":"+nonce+":00000001:"+cnonce+":"+qop+":"+ha2) + `"`
		}

		return out + `, response="` + sum(ha1+":"+nonce+":"+ha2) + `"`
	}
}

// qopOf picks auth from the list a device offers. auth-int is not supported.
func qopOf(offered string) string {
	for option := range strings.SplitSeq(offered, ",") {
		if strings.TrimSpace(option) == "auth" {
			return "auth"
		}
	}

	return ""
}

// digestFields reads the quoted parameters out of a challenge.
func digestFields(challenge string) map[string]string {
	fields := make(map[string]string, digestFieldCount)

	rest := strings.TrimSpace(challenge)
	if space := strings.IndexByte(rest, ' '); space >= 0 {
		rest = rest[space+1:]
	}

	for _, pair := range splitOutsideQuotes(rest) {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}

		fields[strings.ToLower(strings.TrimSpace(key))] =
			strings.Trim(strings.TrimSpace(value), `"`)
	}

	return fields
}

// splitOutsideQuotes splits a challenge on the commas that separate its
// parameters, and not on the commas inside a quoted value such as qop.
func splitOutsideQuotes(rest string) []string {
	var (
		out    []string
		field  strings.Builder
		quoted bool
	)

	for i := range len(rest) {
		switch {
		case rest[i] == '"':
			quoted = !quoted

			field.WriteByte(rest[i])
		case rest[i] == ',' && !quoted:
			out = append(out, field.String())
			field.Reset()
		default:
			field.WriteByte(rest[i])
		}
	}

	return append(out, field.String())
}

func sum(value string) string {
	digest := md5.Sum([]byte(value)) //nolint:gosec // RFC 7616 fixes MD5 here.

	return hex.EncodeToString(digest[:])
}

// nonceValue is the random client nonce a qop=auth digest needs.
func nonceValue() string {
	var raw [8]byte

	_, _ = rand.Read(raw[:])

	return hex.EncodeToString(raw[:])
}

// readSession reads the session id and the timeout the device declared.
func readSession(header string, fallback time.Duration) (string, time.Duration) {
	id, rest, _ := strings.Cut(header, ";")
	id = strings.TrimSpace(id)

	for part := range strings.SplitSeq(rest, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || strings.ToLower(key) != "timeout" {
			continue
		}

		if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
			return id, time.Duration(seconds) * time.Second
		}
	}

	return id, fallback
}

// readChannel reads the interleaved channel the device chose. It may differ
// from the one asked for.
func readChannel(transport string) byte {
	for part := range strings.SplitSeq(transport, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || strings.ToLower(key) != "interleaved" {
			continue
		}

		first, _, _ := strings.Cut(value, "-")
		if channel, err := strconv.Atoi(strings.TrimSpace(first)); err == nil && channel >= 0 && channel < 256 {
			return byte(channel)
		}
	}

	return 0
}

// absolute resolves a track's control URL against the session's base.
func absolute(base, control string) string {
	switch {
	case control == "" || control == "*":
		return base
	case strings.HasPrefix(control, "rtsp://") || strings.HasPrefix(control, "rtsps://"):
		return control
	default:
		return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(control, "/")
	}
}

// stripped returns the URL without its userinfo. The URL goes into every
// request line.
func stripped(parsed *url.URL) string {
	clean := *parsed
	clean.User = nil

	return strings.TrimSuffix(clean.String(), "/")
}

// opOf names what failed without naming the address it failed against.
func opOf(err error) string {
	if opErr, ok := errors.AsType[*net.OpError](err); ok {
		return opErr.Op
	}

	return "reach"
}

// streamError reports a read failure without the address net puts in it.
func streamError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return io.EOF
	}

	return fmt.Errorf("read from the camera: %s", opOf(err)) //nolint:err113 // the cause carries the address.
}
