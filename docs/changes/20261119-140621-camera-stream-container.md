### Fixed
- An RTSP camera never played in the browser when the server ran in a container, because the browser was offered only addresses inside the container. Set `core.webrtc.udp_port` and `core.webrtc.addresses`, then publish that UDP port, and the stream reaches the browser.
