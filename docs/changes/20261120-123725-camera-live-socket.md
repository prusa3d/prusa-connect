### Added
- A camera's live view now plays when the browser cannot reach the server over WebRTC, through a websocket on the page's own port. That covers a tunnel that carries no UDP and a container with no UDP port published, and it needs no configuration.
