# EasyPrint

A Prusa Connect module that is only a browser bundle: the host verifies it
against a digest, serves it, and never runs it.

EasyPrint is the Prusa slicer that runs in a browser, on the Printables site.
It sends a print to a Prusa Connect server from the browser of the person who
uses it. This module is what lets it:

- it asks the server to answer `https://slicer.printables.com`, the origin
  EasyPrint's requests come from, with `allowed_origins` in `module.yaml`. The
  operator approves that at install, and the server answers the origin while
  the module is switched on
- it adds an **EasyPrint** tab to its own page under Settings, Extensions. The
  tab has the link **Add this server to EasyPrint**, which opens
  `https://www.printables.com/slice?pair_connect_local=<address>` in a new tab

The address in the link is the one the person's browser opened the web app on,
because EasyPrint calls the server from that same browser. EasyPrint then
asks the server for a credential through the app pairing, and the person
approves it. The module gives EasyPrint no credential and no permission.

The tab warns in two cases. The first is a server that does not answer the
origin, which is what an extension switched on without an approved install
gets. The second is a page opened over plain HTTP.

## HTTPS

EasyPrint is an HTTPS page. Every browser lets it call a server that answers
over HTTPS with a certificate the browser trusts. To serve the API that way,
set `core.api.tls.certificate` and `core.api.tls.key`, or put a reverse proxy
that holds the certificate in front of the server.

A server on plain HTTP works in fewer browsers, and only on an address of the
local network. Chrome asks the person for permission to reach devices on the
local network, and then lets EasyPrint call the server. Another browser may
refuse the call, so HTTPS is the choice that works everywhere.

## What it needs

A Prusa Connect that reads `allowed_origins` in a manifest. An older server
refuses the manifest when it reads it.

| path | purpose |
| --- | --- |
| `module.yaml` | the manifest: the kind and the version, the origin it asks for, and the `ui` block with the one tab this bundle registers |
| `CHANGELOG.md` | what an operator sees change when they upgrade this module |
| `Makefile`, `scripts/check.sh` | the gate `make -C extensions check EXT=easyprint` runs |
| `install.sh` | builds the bundle and installs it into a scratch core, approved |
| `.vscode/extensions.json` | the editor extensions for the languages in here |
| `ui/src/index.ts` | the bundle's entry: it receives the host object and registers the tab |
| `ui/src/settings.svelte` | `easyprint`: the tab on the module's page under Settings, Extensions |
| `ui/src/easyprint.ts` | the origin, the Printables page and the address that starts the pairing |
| `ui/src/logo.svelte` | the Printables mark the tab draws |
| `ui/src/styles.css` | this module's own Tailwind. Four lines, and only one emits |
| `ui/src/locales/en.json` | every text this module draws. No component writes an English string |
| `ui/src/locales/cs.json` | the Czech catalogue; a key left out falls back to English |

## Installation

To see it, run this in the checkout. `-dir` is this folder:

```bash
go run ./cmd/new-module -start demo -dir extensions/easyprint
```

It starts the demo on a free port with this module installed, and this
module's dev server in front of it. It prints the address to open and the
account to sign in with. `-start app` starts the real app instead, which asks
for an account on its first run. Ctrl+C stops everything it started.

On a server of your own, build the release and install it approved:

```bash
cd extensions/easyprint/ui && pnpm install --frozen-lockfile && pnpm run build
prusa-connect module add github.com/prusa3d/easyprint@0.1.0 -from extensions/easyprint/ui/dist -approve
```

Without `-approve` the declaration is written switched off, and the command
prints `origin https://slicer.printables.com` among what the module asks for.

## Running the tests

From the repository root:

```sh
make -C extensions check EXT=easyprint
```

It lints, type-checks and tests the bundle in Chromium, then builds the
release and installs it with `install.sh`, the same way a person does. The
last step reads the declaration the install wrote and holds it to the origin.
`pnpm run check` in `ui/` runs the first half alone.

## The ceiling

`contributes` in `module.yaml` is the whole of what this bundle may register.
A `host.register` call the block does not declare throws, and the host drops
this module whole.

- `settings: easyprint` ← `host.register.settings('easyprint', Settings)`

## Texts

Every text a person reads is a key in `ui/src/locales/en.json`, read with
`host.t('<key>')`. Copy the file to `<tag>.json` to translate it.

## Troubleshooting

| what you see | what it means |
| --- | --- |
| EasyPrint says it cannot reach the server | the server does not answer the origin. Open the EasyPrint tab: it says so when the origin is missing |
| the tab says the server does not answer EasyPrint | the module was switched on without an approved install. Install it again with approve |
| the browser blocks the call from EasyPrint | the server is on plain HTTP. Read [HTTPS](#https) |
| a screen shows a key instead of a sentence | the key is missing from `ui/src/locales/en.json` |
