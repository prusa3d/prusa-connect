---
title: "ADR-20261124-tep: An MCP server is a module on its own port"
description: "The MCP server is a first-party Go module in extensions/mcp that listens on a port of its own, passes the caller's bearer token to the core REST API, and offers prusa-mcp's tools for what the core has."
date: 2026-11-24
status: proposed
supersedes: []
amends:
  - ADR-20260901-atx
tags:
  - adr
  - modules
  - extensions
  - security
---
# ADR-20261124-tep: An MCP server is a module on its own port

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md) | Builds on [ADR-20260910-he3](20260910-he3-documentation-is-written-for-people-and-held-to-the-code.md) rule 18, the MCP server as a module, and [ADR-20261019-3ns](20261019-3ns-a-module-declares-its-ports-and-gets-a-data-directory.md), the port grant | Amends [ADR-20260901-atx](20260901-atx-durable-command-records.md) rule 3, the payload of the `authorized` frame

An AI assistant on the local network, such as Claude Code or Cursor, drives
printers through an MCP server. Prusa Connect in the cloud has one already,
prusa-mcp. A person who uses both should find the same tools under the same
names. Beta idea CL-142 asks for such a server. A module route cannot
carry MCP: it buffers the body, ends after ten seconds, and drops the
`Authorization`, `Origin` and `Mcp-*` headers. So the server is a module with a port of its own, and the core checks every call it
makes.

## rules

### where it runs

1. The MCP server is the extension `extensions/mcp`, a Go module that runs as
   a service. It is versioned and shipped like the other first-party
   extensions, and the core does not import it.
2. It serves MCP over streamable HTTP at `/mcp` with the official Go SDK,
   `github.com/modelcontextprotocol/go-sdk`, in its stateless mode.
3. It asks for one TCP port in `needs.listen`: name `mcp`, port `8090`, why
   "AI assistants (MCP clients) on this network". The operator grants it with
   `-approve-listen` and may move it with `-listen mcp=<port>`.

### who the caller is

4. Every request carries `Authorization: Bearer <token>`, a core API
   credential. The module refuses a request without one with `401` and
   keeps no credential of its own.
5. The module calls the core REST API at its option `core_url`, default
   `http://127.0.0.1:8080`, with the caller's token, for every tool. It
   reaches another module's routes through the core mount
   `/api/v1/modules/<namespace>/` the same way. It never calls the core with
   the module's own identity on behalf of an MCP caller.
6. The module reads the token's scopes from `GET /api/v1/principal`. A token
   without `operate` gets only the tools that change nothing, and the server
   instructions then say that the token is read-only.
7. A person gets a token from the page "Connect an AI client" in the
   module's UI. It mints a credential named `MCP: <client>` through
   `host.client` with the person's session. The scope is `read` unless the
   person picks `operate`, and the credential is valid for 90 days unless
   the person picks 30 days or a year. It shows the token once, with the command or snippet for the
   client, and keeps it nowhere. The person revokes it under Settings, API
   credentials.

### what it offers

8. A tool that prusa-mcp has and the core can serve keeps prusa-mcp's name,
   its arguments and the shape of its answer. It drops `organization_id`,
   because this server has one organization.
9. Every tool is registered whenever the scope allows it. There is no switch
   per tool group.
10. Every tool declares the MCP annotation `readOnlyHint`, and a tool that
    ends a print declares `destructiveHint`. The server instructions tell the
    assistant to say what a tool that is not read-only will do and to get the
    person's go-ahead before it calls one. A tool takes no `confirm`
    argument. Whether a person approves each call is a setting of the MCP
    client, and the page of rule 7 says so next to the snippet.
11. Tools are registered from one list in the module. A later mechanism by
    which another module contributes tools, including a module outside this
    repository, adds to that list and needs its own record.

### the port

12. The port speaks plain HTTP unless the options `tls_certificate` and
    `tls_key` name a certificate, as `core.api.tls` does for the core. With
    them it speaks TLS 1.3 and nothing older.
13. The port listens on every interface unless the option `bind` names an
    address.
14. The SDK's protection against DNS rebinding stays on, and the handler is
    wrapped in `http.NewCrossOriginProtection`, so a cross-origin request
    from a browser is refused with `403`.

### audit

15. The `authorized` frame of a command names the api credential the caller
    presented, as `credential`, its public id, and `credential_name`, the
    name it was minted under. Both are empty for a session. The name is
    copied into the frame so that the record still says it after the
    credential is revoked. This adds two optional fields to
    `CommandAuthorized` of
    [ADR-20260901-atx](20260901-atx-durable-command-records.md) rule 3.
    The command log of a printer shows the name next to the principal, so
    the log names `MCP: <client>` for a command an assistant sent.
16. Every call of a tool that is not read-only writes one log line at info
    with `audit=true`: the tool, the credential's public id, the principal,
    and the outcome. It never logs the token.

## enforcement

- Rules 3, 12 and 13: a manifest test in `extensions/mcp` holds `needs.listen`
  and the option defaults to this record.
- Rules 4, 5 and 6: an integration test in `extensions/mcp` runs the module
  against a core. It calls `tools/list` with a `read` and an `operate`
  token, and calls a write tool with a `read` token.
- Rule 8: a test holds the tool names and argument names to a list copied
  from prusa-mcp.
- Rule 10: a test fails for a tool registered without `readOnlyHint`.
- Rule 14: a test sends a cross-origin `POST` and expects `403`.
- Rule 15: a core test sends a command with an api credential and with a
  session, and reads `credential` and `credential_name` from the
  `authorized` frame. A component test renders the name in the command log.
- Rule 16: a test calls a write tool and reads the audit line.
- Rules 1, 7, 9 and 11 have no gate.

## rejected

| option | why not |
|---|---|
| MCP in the core | it grows the core against the small-core rule and adds about seven modules to its `go.mod` |
| MCP through `routes.serve` | the route buffers 1 MiB, ends after ten seconds, and drops `Authorization`, `Origin` and `Mcp-*` |
| a streaming `routes.serve` | a change to the wire protocol and `sdk/` for one module |
| the module in a repository of its own | it calls the core REST API, and an API change and the tool that wraps it should land in one merge request |
| `routes.call` over the wire | it carries a person only inside a request the host serves, so a call from the module's own port would act as the module |
| MCP OAuth for local clients | clients need HTTPS for the authorization server, rare on a LAN, and a bridge to app pairing is 1500 lines |
| app pairing started from the client | no MCP client speaks the device flow |
| list every tool and answer `403` for a read-only token | the assistant offers what it cannot do; a shorter list says so before the first call |
| a person approves each write in the web app, as a pending command | the MCP client can ask a person before each tool call, and a pending-command queue is new core work for one module |
| a `confirm=true` argument on writes | it changes the tools' shapes from prusa-mcp's |
| switches per tool group, as prusa-mcp has | the tools are few, and a switch is a setting nobody needs yet |
| a certificate the module makes itself | clients do not trust it without a manual step, and the core has no such machinery either |
| a client generated from the OpenAPI document | a hand-written client for thirteen tools is smaller than the generator setup |

## open items

- `GET /api/v1/principal` does not answer `scopes` yet. Rule 6 needs it.
- `CommandAuthorized` has no `credential` or `credential_name` yet. Rule 15
  needs them.
- Remote access, MCP OAuth and HTTPS for remote connectors are left to a later record.
