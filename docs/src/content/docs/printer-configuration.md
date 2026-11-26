---
title: Printer Configuration
description: Configure a Prusa printer to connect to Prusa Connect via INI file on USB.
audience: run
rewrite:
  describes: current
  implementedBy:
    - scripts/generate_printer_config.sh
    - examples/prusa_printer_settings.ini.example
    - internal/connector/xbuddy
---

A Prusa printer finds this server through a settings file on a USB drive. The
file names the server's address and never a token. The printer gets its token
from this server when you choose **Add Printer to Connect** on the printer and
its code is approved.

Printers dial the API listener, `:8080` by default, the same port the web app
uses. They dial another port only when the printer connector has a listener of
its own (`core.connectors.<name>.config.listener.addr`).

If you need `tls = true` with a private CA, see [Custom Certificates](./certificates.md).

A printer whose firmware can pair joins without a USB drive. See
[Pair a printer at its screen](./pair-a-printer-at-its-screen.md).

## What the printer sends, and when

The public Buddy firmware does this, in `src/connect/`:

- With no host or no token stored, the printer sends nothing and its status is
  `No Config`.
- With both stored, it sends one request, `GET /p/ws`, a WebSocket upgrade with
  the headers `Fingerprint`, `Token` and `Sec-WebSocket-Protocol: prusa-connect`.
  It never calls `/p/register` on its own.
- **Add Printer to Connect** in its menu sends `POST /p/register`. The printer
  shows a code and asks again until the code is approved. Then it stores the
  token the server issued and switches Connect on.

So a printer reaches the **Add a printer** page only through **Add Printer to
Connect**. A printer that dials `/p/ws` with a token the server did not issue
is refused. The page and the notification bell say so, with the reason.

## Add a printer with a USB drive

:::tip
Is the printer in Prusa Connect now? Download its configuration file there
before you start. That file brings the printer back to Prusa Connect later
with its registration, see [Move a printer back to Prusa Connect](#move-a-printer-back-to-prusa-connect).
:::

### Step 1: create the INI file

Create `prusa_printer_settings.ini` in the root folder of a USB flash drive. The
**Add a printer** page of the web app offers this file with your server's
address filled in, as its first step.

```ini
[service::connect]
hostname = 192.168.1.100
port = 8080
tls = false
```

| Parameter | Description |
|-----------|-------------|
| `hostname` | IP address or hostname of your server. Length is capped, see [Hostname length](#hostname-length) |
| `port` | The port printers dial: `8080` unless the connector has a listener of its own. See [Ports and TLS](#ports-and-tls) |
| `tls` | `true` or `1` for WSS (encrypted), `false` or `0` for WS |
| `token` | Not in the file. The printer stores the token this server issues when its code is approved |

The file loads with `hostname` alone. Loading a file with no `token` line
clears the token the printer held.

### Step 2: load the file on the printer

1. Insert the USB flash drive
2. **Settings > Network > Prusa Connect > Load Settings**
3. The printer answers _"Loaded successfully. Connect will activate shortly."_
   If it says _"Enable Connect to activate"_ instead, switch Connect on. You
   do not need to switch Connect off and on otherwise

### Step 3: ask to join

On the same screen choose **Add Printer to Connect**. If the printer says
_"Previous registration to Connect will be forgotten. Proceed?"_, choose
**Yes**. The printer shows a code.

- When the server asks you first, the printer appears on the **Add a printer**
  page within seconds, without a reload. Check that the code and the
  fingerprint match the printer's display, and choose **Approve**.
- When the server admits every printer, it approves the code at once, and the
  page shows the printer joining.

The printer then stores its token and switches Connect on.

### Step 4: check the status

In **Settings > Network > Prusa Connect**:

- **Status:** Online when the printer is connected
- **Error:** what went wrong otherwise, see [Status and error messages](#status-and-error-messages)

## Which printers the server admits

The printer connector's `core.connectors.<name>.config.mode` decides which
credential a printer must present on `/p/ws`. The first-run wizard writes it as
the admission answer: `open` is `none`, and `ask` is `token`.

| `mode` | a printer on `/p/ws` is accepted when |
|---|---|
| `none` | it sends a fingerprint. The token is not checked |
| `token` | its token is one this server issued to that printer on approval |
| `fingerprint` | its fingerprint belongs to an approved registration |
| `both` | both of the above hold |

## What removing a printer does

**Remove this printer** on the printer's page takes the printer off the server
and takes away its access. The server forgets the token and the approved
fingerprint it issued that printer. The prints and the command log stay
readable under the old record, and nothing brings that record back.

- In mode `token`, `fingerprint` or `both`, the printer is refused with 401
  the next time it connects, and shows `Unauthorized`. To bring it back,
  choose **Add Printer to Connect** on the printer and approve the new code.
  It joins as a new printer.
- In mode `none` the server checks no credential, so a removed printer that is
  still switched on joins again by itself as a new printer.

## Move a printer back to Prusa Connect

A printer talks to one Connect server at a time. Loading the settings file of
this server replaces the Prusa Connect address on the printer, and asking to
join replaces its Prusa Connect token. To move back, load a settings file that
names Prusa Connect again.

### With the configuration file from Prusa Connect

In Prusa Connect, the settings of each printer offer its configuration file,
`prusa_printer_settings.ini`. Its `[service::connect]` section names Prusa
Connect and holds the printer's token. The file is the printer's credential, so
keep it private and do not share it.

1. Download the file on [connect.prusa3d.com](https://connect.prusa3d.com).
   Do this before you add the printer here, or later, while the printer is
   still in your Prusa Connect account
2. Copy it to the root folder of a USB flash drive
3. On the printer, choose **Settings > Network > Prusa Connect > Load Settings**

The printer loads the address and the token and goes online in Prusa Connect.
It keeps its registration there, so you do not register it again.

### Without the file

Write the settings file yourself. The address printers use is
`buddy-a.connect.prusa3d.com`, not the address of the website:

```ini
[service::connect]
hostname = buddy-a.connect.prusa3d.com
port = 443
tls = true
```

Load it as above. The file has no `token`, so the printer clears the token it
holds. Then choose **Add Printer to Connect** on the printer and register it on
[connect.prusa3d.com](https://connect.prusa3d.com) with the code it shows.

To take the printer off this server too, see
[What removing a printer does](#what-removing-a-printer-does).

## Generate the INI automatically

```bash
make generate-printer-config
```

Reads `config/config.yaml` and writes `prusa_printer_settings.ini` with the
port and the TLS setting of the listener printers dial. It writes no token.

## Troubleshooting

### Printer won't connect

On a direct ethernet cable to a Windows machine, run
[the bench preflight](./bench-preflight.md) first. It separates the cable, the
address, the firewall, the bind address and which server holds the port, and
each of those looks identical from the printer's side.

1. Check backend is running: `make status`
2. Verify network, printer must reach the server (check firewall, same network)
3. Check TLS, for testing use `tls = false`; for production use a valid certificate
4. Check logs: `make logs`. The server writes a warning when it refuses a
   printer, see [What the server logs](#what-the-server-logs)

### Status and error messages

| Printer shows | Cause | Fix |
|-------|-------|-----|
| `No Config` | The printer holds no host or no token, so it sends nothing | Load the INI file, then choose **Add Printer to Connect** |
| `Unauthorized` | The server refused `/p/ws` with 401 or 403: an unknown token, a removed printer, or a printer it would not admit | Choose **Add Printer to Connect** and approve the code |
| `Srv error` | The server answered `/p/ws` with another status than 101, 400, 401 or 403, such as 404 | See [Srv error](#srv-error) |
| `Bug` | The server answered `/p/ws` with 400 | Check the server log for the request |
| `DNS error` | The hostname does not resolve, or it was too long and the printer stored a cut-off version of it | Check the length against [Hostname length](#hostname-length), then that the name resolves |
| `TLS error` | Certificate problem | Use `tls = false` for testing, or fix the cert |
| `Refused` | Server not running or wrong port | Check `make status` and port config |

### Srv error

The printer shows `Srv error` for any answer to `/p/ws` other than 101, 400,
401 or 403. The usual cause is a 404: no printer connector serves the port the
printer dials. The connector may have a listener of its own on another port,
or another program holds the port. Check the port in the INI file, and that
the printer connector is configured.

### What the server logs

The server writes a warning for each of these cases, at most once a minute:

- a printer refused on `/p/ws` with 401 or 403. The line gives the reason, the
  printer's fingerprint, its network address and the admission mode, once a
  minute for each printer
- a printer that asked for `/p/ws`, `/p/register` or `/p/auth` on a port where
  no printer connector serves it. The line names the path, and the printer is
  answered 404

A refused printer is also an event, `com.prusa.connect.device.registration.refused`,
on the same once-a-minute clock. It carries the reason and the model and
firmware the printer reported, but not its fingerprint or its address. The Add
a printer page and the notification bell show it.

## The hostname the add-printer page writes

The add-printer page writes the INI with `core.frontend.printer_address`, the
origin printers dial, for example `http://192.168.1.50:8080`. The setup wizard
sets it from the address its encryption step asks, which starts from the
machine's first network address when the wizard was opened on `localhost`. It
leaves the key alone when the configuration file declares how printers connect.
With the key empty the page uses the address it was opened on, and warns when
that is a loopback name, because on a printer `localhost` is the printer
itself.

The server refuses to start with a value that is not a bare `http` or `https`
origin. It also refuses a loopback or unspecified hostname, and one longer than
a printer stores. Inside a container the wizard sees the container's address,
not the host's, so check the value it offers there. On a bridge network, enter
the host's address. Without a port the wizard uses the port through which the
browser opened it when printers share the web app's listener. When printers
have a listener of their own, it uses that listener's port. Add a port when
printers reach that listener through another published port, for example
`192.168.1.24:8481`.

## Hostname length

The printer keeps the Connect hostname in 20 bytes plus a terminator, so what
has to fit 20 characters is not the name you type but what the firmware stores.
It shortens two suffixes on the way in, and only those two:

| Hostname ends in | Stored as | Longest name that works |
|------------------|-----------|-------------------------|
| `connect.prusa3d.com` | everything before the suffix, plus 2 bytes | 35, the cap on the INI value itself |
| `prusa3d.com` | everything before the suffix, plus 2 bytes | 29 |
| anything else, an IP included | the name as typed | **20** |

Nothing tells you when a name is too long. The INI loads and reports success,
because it is checked against a 35-byte buffer. The printer's own menu shows
the name in full. Only the stored copy is cut short, and the printer then
reports `DNS error` for a host it never held. `printers.workshop.example.net` is
29 characters and fails this way; `shop.example.net` is 16 and does not.

A name over 29 characters ending in `prusa3d.com` fails the same way, because
it is shortened first and cut short after, which also loses the two bytes that
mark it as shortened.

An IP address is not shortened either, but at 15 characters at most it always
fits. It only works with `tls = false`, or with a certificate that names the
address: the firmware verifies the hostname against the certificate.

## Ports and TLS

- **Default ports:** the firmware dials 443 for TLS and 80 for non-TLS when `port` is missing from the INI. The server listens on `8080` by default, so write the port.
- **TLS:** Always use `tls = true` in production.
