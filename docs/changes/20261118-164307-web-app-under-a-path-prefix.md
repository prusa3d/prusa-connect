### Added
- A reverse proxy listed in `core.trusted_proxies` can serve the web app under a path, such as the Home Assistant Supervisor's Ingress or nginx at `/connect/`. It names the path in `X-Forwarded-Prefix` or `X-Ingress-Path`, and the API and the printers' addresses stay at the server's own root.
