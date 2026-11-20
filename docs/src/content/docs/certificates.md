---
title: Custom Certificates
description: Generate a private PKI for TLS between Prusa printers and the Prusa Connect xBuddy listener.
audience: run
rewrite:
  describes: current
  implementedBy:
    - scripts/generate_certificates.sh
    - internal/connector/xbuddy/listener.go
    - internal/app/config/tls.go
    - internal/app/apitls.go
    - internal/kernel/certfile
---

This guide covers generating a certificate chain that works with Prusa printers and the Prusa Connect xBuddy listener.

Target use case: you run your own backend, want `tls = true`, and use a private CA instead of a publicly trusted certificate.

## How firmware handles TLS

The printer acts as a **TLS client** that verifies the **backend server certificate**. It can load one custom **trusted CA certificate** from USB.

- **Printer:** gets a trusted CA certificate in DER format as `connect.der`
- **Backend:** gets an ECDSA server certificate plus private key in PEM format

There is no client certificate or mTLS flow in this firmware path.

## Firmware requirements

- Certificate must be **ECDSA** (not RSA)
- Backend must support **TLS 1.2** (firmware enforces it)
- Cipher suite: `ECDHE-ECDSA-AES128-GCM-SHA256`
- Certificate SAN must match the hostname configured in the printer
- `connect.der` must be **DER** format (not PEM), one certificate per file
- Root CA must have `basicConstraints=CA:TRUE` - mbedTLS rejects a trust anchor without it
- When the printer connects by IP literal (`hostname=192.168.1.50` in INI), the cert SAN must include **both** `IP:192.168.1.50` and `DNS:192.168.1.50` (see [IP literal hostnames](#ip-literal-hostnames-mbedtls-2x-workaround))

## PKI layout

The chain is intentionally flat - the root CA signs the server certificate directly:

```
Root CA  (offline, long-lived, exported as connect.der for the printer)
  ├── api.crt     (HTTPS / WebSocket API server - browser-facing)
  └── printer.crt (HTTPS / WebSocket xBuddy listener - printer-facing)
```

**Why no intermediate CA?** The printer has little memory. Its TLS stack reads each record into a 1024-byte buffer and cannot join a handshake message that is split over several records. So the whole Certificate message the server sends must fit in 1024 bytes. A typical leaf and intermediate together are about 1.6 KB, and the printer drops the connection before it checks anything. This is a firmware limit and will not change on current printers.

### Certificate size limit

The Certificate message is 7 bytes, plus 3 bytes and the DER size of each certificate the server sends. It must be at most 1024 bytes:

| server sends | fits if |
|---|---|
| the leaf alone | the leaf is at most 1014 bytes in DER |
| the leaf and one intermediate | both together are at most 1011 bytes in DER |

An ECDSA P-256 leaf with short names is about 500 bytes. Check a certificate's DER size with:

```bash
openssl x509 -in certs/server/printer.crt -outform DER | wc -c
```

`connect.der` may be any CA in the chain, the root or an intermediate. The printer trusts the certificate in `connect.der` as its trust anchor. If your private CA has an intermediate that is too large to send:

- serve the leaf alone on the printer-facing listener, and
- export the CA that issued the leaf (the intermediate) as `connect.der`.

That intermediate then has to be copied to every printer again when it is replaced. The flat chain above avoids this.

When the certificates do not fit, the printer shows only "TLS Error". In a packet capture it sends a TCP reset right after `ServerHelloDone`, with no alert.

A reverse proxy in front of the printer listener sends the chain it was given. Many proxies send the full chain by default. Give the printer-facing host a certificate file with the leaf alone.

The hostname the printer dials is limited as well, see [Hostname length](./printer-configuration.md#hostname-length).

## Generate certificates

### Helper Makefile targets (recommended)

```bash
make certificate-all     # Root CA + both server certs + connect.der
make certificate-server  # API cert only
make certificate-prusa   # Printer cert + connect.der only
```

Optional overrides:

```bash
API_CERT_CN=api.local API_CERT_DNS=api.local,localhost make certificate-server
PRUSA_CERT_CN=printer.local PRUSA_CERT_DNS=printer.local PRUSA_CERT_IPS=192.168.1.50 make certificate-prusa
```

### Manual generation with openssl

**Root CA:**

```bash
openssl ecparam -name prime256v1 -genkey -noout -out certs/ca/root-ca.key

openssl req -x509 -new -sha256 \
  -key certs/ca/root-ca.key \
  -out certs/ca/root-ca.crt \
  -days 3650 \
  -subj "/C=CZ/O=Your Company/CN=Your Root CA" \
  -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  -addext "subjectKeyIdentifier=hash"
```

**Server certificate** (repeat for API and printer certs with different CN/SAN):

```bash
openssl ecparam -name prime256v1 -genkey -noout -out certs/server/printer.key

openssl req -new -sha256 \
  -key certs/server/printer.key \
  -out certs/server/printer.csr \
  -subj "/C=CZ/O=Your Company/CN=pcl.home.arpa"

openssl x509 -req -sha256 \
  -in certs/server/printer.csr \
  -CA certs/ca/root-ca.crt \
  -CAkey certs/ca/root-ca.key \
  -CAcreateserial \
  -out certs/server/printer.crt \
  -days 825 \
  -extfile <(printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=DNS:pcl.home.arpa,IP:192.168.1.50\n')
```

**Printer trust file:**

```bash
openssl x509 -in certs/ca/root-ca.crt -outform DER -out certs/printer/connect.der
```

## Configure the core

`prusa-connect core` binds one listener by default, plain HTTP, carrying
the whole origin including `/p/register` and `/p/ws`. Giving the xBuddy
connector a listener address adds a second socket with its own certificate.

```yaml
core:
  api:
    addr: ":8080"
  connectors:
    xbuddy:
      type: xbuddy
      config:
        listener:
          addr: ":443"
          tls:
            certificate: "certs/server/printer.crt"
            key: "certs/server/printer.key"
          # handshake_timeout: 60s
```

`xbuddy` there is an instance name you choose. Write no `connectors` block at
all and the core mounts the shipping xBuddy dialect on its own defaults. Its
listener address is then empty, so there is no second socket and printers dial
the API listener.

| key | what it does |
|---|---|
| `core.connectors.<name>.config.listener.addr` | the device-facing socket. Empty is the default: printers dial `core.api.addr` over plain HTTP |
| `core.connectors.<name>.config.listener.tls.certificate` / `.key` | PEM leaf and key. Must be ECDSA; the server refuses an RSA certificate at startup, naming the cipher suite the firmware offers |
| `core.connectors.<name>.config.listener.handshake_timeout` | how long a printer is given to complete the handshake, default 60s. The API listener's 10s is not enough for a CORE One running phase stepping |
| `core.api.tls.certificate` / `.key` | the certificate a browser verifies. Empty is plain HTTP |

What each listener serves and refuses:

| listener | serves | TLS versions | cipher suites | ALPN |
|---|---|---|---|---|
| `core.api.addr` | the whole origin: API, OpenAPI document, `/docs`, the frontend, and the device wires | 1.3 and above | Go's defaults | `h2`, `http/1.1` |
| the connector's `listener.addr` | the device wires alone (`/p/register`, `/p/ws`). Everything else is a 404 problem document | 1.2 exactly | `TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256` only | none |

The printer listener has a ceiling as well as a floor. Go ignores
`tls.Config.CipherSuites` under TLS 1.3, so a listener that allows 1.3 offers
whatever Go's 1.3 suites are and the single-suite pin above becomes a claim
rather than a configuration. This socket carries two routes to one client whose
profile is frozen in shipped firmware, so the ceiling costs nothing that dials
it. Anything that is not a printer belongs on `core.api.addr`, which starts at
1.3.

Two listeners because one socket cannot satisfy both clients: a printer verifies
against one CA certificate loaded from a USB stick and speaks TLS 1.2 with a
single ECDSA suite, and a browser verifies against its own trust store. One
socket makes the printer's floor everybody's floor, presents one certificate to
two trust stores, and answers the frontend and the whole API on whatever network
the printers are on.

Neither listener has a minimum-version key. Who dials decides the floor, and a
key with one correct value is a key somebody eventually gets wrong.

The core reads the certificate it was given and says so at startup:

```
level=INFO msg="the printer listener serves TLS" certificate=certs/server/printer.crt
  min_version=1.2 max_version=1.2 cipher_suite=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
  dns_names="[prusa-connect.local 127.0.0.1]" ip_addresses=[127.0.0.1] not_after=2028-11-28
  issued_by="CN=Your Root CA,O=Your Company,C=CZ"
level=INFO msg="serving devices" printer=127.0.0.1:443 tls=true handshake_timeout=1m0s
  routes="[/p/register /p/ws]"
```

`issued_by` is the subject a printer's `connect.der` must carry. Comparing the
two is the whole of the wrong-CA check, and it needs no printer:

```bash
openssl x509 -inform DER -in certs/printer/connect.der -noout -subject
```

A certificate carrying an IP address with no `DNS:` twin still serves, and warns:

```
level=WARN msg="the printer certificate carries an IP address with no matching DNS name,
  so a printer configured by that address will fail the handshake with 0x04"
  address=192.168.1.50 missing=DNS:192.168.1.50
```

## What the core refuses at startup

Each of these is checked before a port is bound, so the process does not start
rather than starting and serving every printer a refusal.

| refused | message names |
|---|---|
| an RSA certificate in the connector's `listener.tls.certificate` | the one cipher suite the firmware offers |
| a `listener.tls.certificate` past its `notAfter` | the expiry date and the clock this machine reads |
| a `listener.tls.certificate` before its `notBefore` | the same two, which on a machine with no battery-backed clock is the clue that the clock is wrong and the certificate is not |
| a PEM chain written root-first | which certificate in the file the key actually signs for |
| `core.api.addr` and the connector's `listener.addr` on one socket | both keys. Linux refuses the second bind; Windows binds both and answers the whole API in the clear on the printer's port |

A certificate inside 30 days of expiry still serves, and warns at startup with
the date and what is left.

## Replace a certificate without a restart

Both listeners read their certificate and key again when either file
changes. A renewal tool such as certbot can write the new files in place.
Within about a second, the next handshake presents the new certificate.
Connections that are already open keep the certificate they started with, and
no printer is disconnected.

| what happens | what the log says |
|---|---|
| the new pair loads | the line `the API listener serves TLS` or `the device listener serves TLS` again, with the new expiry date |
| the new pair does not load | one warning that the listener kept the certificate it had, with the reason |

A renewal writes two files, so each listener watches both. A certificate that
arrives before its key does not match that key, and the listener takes the
pair when the key arrives. The printer listener also refuses a new
certificate it would refuse at startup, such as an RSA one, and keeps the one
it had.

## Configure the printer

USB flash drive contents:
- `prusa_printer_settings.ini`
- `connect.der`

The page that adds a printer offers both files for download. It offers
`connect.der` when `core.frontend.printer_custom_ca` is on and the printers
connect over HTTPS. The server reads it from the `certs` folder of its
configuration directory. If you made the certificates yourself, export the
certificate of your own authority as a DER file and copy it to the drive as
`connect.der`.

```ini
[service::connect]
hostname = pcl.home.arpa
port = 443
tls = 1
custom_cert = 1
```

Load on printer: **Settings - Network - Prusa Connect - Load Settings**, then
choose **Add Printer to Connect** and approve the code on the server. The
printer stores the token the server issues.

## Validation

```bash
# Verify certificate is signed by the CA
openssl verify -CAfile certs/ca/root-ca.crt certs/server/printer.crt

# Test live TLS handshake
openssl s_client -connect pcl.home.arpa:443 \
  -servername pcl.home.arpa \
  -tls1_2 -showcerts

# Verify DER file
openssl x509 -inform DER -in certs/printer/connect.der -noout -text
```

## Troubleshooting

| Symptom | Likely cause |
|---------|-------------|
| `ssl handshake failed with: -68` on printer | Chain validation failed - CA cert missing `basicConstraints=CA:TRUE`, or SAN mismatch |
| `ssl handshake failed with: -9984` (verify_result `0x04`) | CN/SAN mismatch, printer is connecting by IP literal and the cert SAN only has `IP:` without `DNS:` (see below) |
| `ssl handshake failed with: -9984` (verify_result `0x08`) | Chain not trusted, `connect.der` on the printer does not match the CA that signed the server cert. Re-run Load Settings with the current `connect.der`. |
| TLS error on printer | RSA cert instead of ECDSA; TLS 1.2 not supported; cipher suite mismatch |
| `ssl handshake failed with: -28928` or `-29184` | the certificates sent do not fit in 1024 bytes, see [Certificate size limit](#certificate-size-limit) |
| Works with `tls = false`, fails with `tls = true` | Hostname in `prusa_printer_settings.ini` does not match SAN in `printer.crt` |
| Printer configured with IP | SAN must include both `IP:x.x.x.x` *and* `DNS:x.x.x.x` (see below) |

**`connect.der` must be actual DER**, not a PEM file renamed to `.der`. One X.509 certificate per file.

## Reading a failed handshake from the core's log

A printer that refuses the certificate it was presented sends a fatal alert
saying why, and mbedTLS picks a different alert per reason. The core logs every
one of them through the configured logger, at `WARN`, naming the listener and
the address the printer dialled from:

```
level=WARN msg="http: TLS handshake error from 192.168.1.71:49212: remote error:
  tls: unknown certificate authority" listener=printer
```

| the line ends with | the printer is saying | fix |
|---|---|---|
| `unknown certificate authority` | it does not trust the CA that signed what it was shown | `connect.der` on the USB stick is not the CA in `issued_by` on the startup line. Re-export it and Load Settings |
| `bad certificate` | the chain verified and the name did not | `hostname` in `prusa_printer_settings.ini` is not in `dns_names` on the startup line. For an IP literal, the SAN needs `DNS:<ip>` as well as `IP:<ip>` |
| `expired certificate` | the certificate's validity window has passed | reissue. The core refuses to start on one that has already expired, so this is a certificate that expired while it was running |
| `client offered only unsupported versions` | it speaks a TLS version this listener does not | this listener is TLS 1.2 exactly. A client that is not xBuddy firmware belongs on `core.api.addr` |
| `client offered: [...]` | it offers no cipher suite this listener does | the certificate is RSA, or the client is not a Prusa printer |
| `connection reset by peer` or `EOF` | it dropped the connection without an alert | the certificates sent are larger than the printer can read. See [Certificate size limit](#certificate-size-limit) |
| nothing at all | no packet arrived | cable, firewall, address, or another server holding the port. [Bench preflight](./bench-preflight.md) |

The printer's own hostname is the one thing no alert carries, because the
printer has stopped talking by the time the alert arrives. The listener reads it
out of the ClientHello instead, and says so before the printer refuses:

```
level=WARN msg="a printer opened a handshake this certificate cannot answer, so it
  will refuse what it is sent" certificate=certs/server/printer.crt
  server_name=192.168.1.50 remote=192.168.1.71:49212
  reason="certificate is not valid for requested server name: ..."
  dns_names="[prusa-connect.local]"
```

`server_name` is what `hostname` in `prusa_printer_settings.ini` was set to, and
it is the exact string that has to appear as a `DNS:` entry on the certificate.

`core.log.format: json` renders the same fields as JSON, because the line goes
through the configured handler rather than to raw stderr.

## IP literal hostnames (mbedTLS 2.x workaround)

xBuddy firmware uses mbedTLS 2.x. Its hostname verification compares the string passed to `mbedtls_ssl_set_hostname()` byte-for-byte **only against DNS SAN entries**, `IP:` SAN entries are not matched against an IP-shaped hostname string. Without a `DNS:<ip>` duplicate in the cert, a printer configured with `hostname=192.168.1.50` in `prusa_printer_settings.ini` fails the handshake with `MBEDTLS_X509_BADCERT_CN_MISMATCH` (`verify_result = 0x04`), even though the cert is correctly signed and the IP literal matches the `IP:` SAN entry. mbedTLS 3.x handles IP-string matching natively; Buddy is currently on 2.x.

`make certificate-prusa` (via `scripts/generate_certificates.sh`) automatically emits both forms for every IP given in `PRUSA_CERT_IPS`:

```
subjectAltName=IP:192.168.1.50,DNS:192.168.1.50
```

`DNS:<ip>` is technically not a valid DNS name per RFC 5280, but CA/Browser Forum has historically tolerated it and string equality is the only check done by mbedTLS 2.x, so it works.

**When to skip the workaround:** if you configure the printer with a real DNS hostname (`printer.lan`, `prusa-connect.local` with mDNS, `connect.example.com` with a real DNS server) and set `hostname=<that-name>` in the INI, you only need a `DNS:<name>` SAN. No IP fallback needed.

**Practical recommendation:** prefer DNS hostnames (mDNS / local DNS) over IP literals when possible, IPs change with DHCP and you would need to re-generate the cert and Load Settings on every printer.
