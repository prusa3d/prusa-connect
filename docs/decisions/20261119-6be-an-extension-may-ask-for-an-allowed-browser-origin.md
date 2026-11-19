---
title: "ADR-20261119-6be: An extension may ask for an allowed browser origin"
description: "The allowed_origins request: an extension an operator approved adds a browser origin to the ones the API answers while it is switched on, and Settings keeps a list of its own beside the configuration file."
date: 2026-11-19
status: proposed
supersedes: []
amends: []
tags:
  - adr
  - modules
  - security
---
# ADR-20261119-6be: An extension may ask for an allowed browser origin

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md) | Related: [ADR-20261021-x2b](20261021-x2b-a-module-may-list-the-people.md)

Some extensions work with a page of another site. A slicer that runs in a
browser on its own site sends a print to this server from that browser. The
browser lets the page read an answer only when the server names the page's
origin, which is its scheme, host and port. The server named an origin only
when `core.api.allowed_origins` in the operator's file listed it, and it read
that file once, at start.

So an operator had to learn the address, edit a file and restart. This record
gives the list two more sources: a list a person keeps in Settings, and the
extensions that are switched on.

An allowed origin is a wide grant. Its page reads every answer the API gives
to the credential the browser sends with the request. The session cookie is
`SameSite=Lax`, so a browser sends it from a page of another site only when
that site shares this server's registrable domain. A page of any other site
acts with a credential it was given, such as one from the app pairing.

## rules

### what the extension declares

1. An extension asks with the manifest key `allowed_origins`, a list of at
   most 8 origins. An extension with no process may ask too.
2. Each entry is an exact `https` origin as a browser writes it: lower case,
   a port only when it is not 443, and nothing after the host. A wildcard, a
   path and an `http` origin are refused when the manifest is read.

### who grants it

3. The key is a request, like `permissions`. `module add` and
   `POST /api/v1/modules` write it into the declaration only with approve, as
   `module.allowed_origins`, and record the ask as
   `module.asked.allowed_origins`.
4. Approving it takes the admin role on the organization, as approving a
   permission does. `configure_resource` on the resource kind is not enough.
5. An install without approve lists each entry among the requests, as
   `origin <origin>`, and leaves the extension switched off. Switching it on
   later without an approved install grants no origin.
6. A marketplace row and a module index row carry the key as
   `allowed_origins`. An approved install whose manifest asks for an origin
   the operator was not shown is refused with `MODULE_REQUESTS_CHANGED`.
7. An approved update keeps what the operator decided. An origin missing from
   `module.allowed_origins` and present in the recorded ask stays away. An
   origin the new release adds is granted and named in `added`.

### what the grant does

8. The server answers a granted origin exactly as it answers one from
   `core.api.allowed_origins`, while the extension is switched on.
9. The origin leaves with the write that switches the extension off or removes
   it, with no restart. It also leaves while `core.modules_enabled` is false
   and in safe mode.
10. The origin is never copied into `core.api.allowed_origins` or into the
    Settings list. It is read from the declaration, so a removed extension
    leaves nothing behind. An origin another source also names stays allowed
    by that source.
11. The grant writes nothing into the authorization graph and gives the
    extension's process nothing. A page of the origin still needs a
    credential.

### how it shows

12. `GET /api/v1/allowed-origins` lists every allowed origin with its source:
    `file`, `settings` or `extension`. An extension's origin carries the
    namespace and the title of the extension.
13. Settings, Server, Network draws that list. A row of an extension names the
    extension and has no remove button: a person switches the extension off.
14. The install confirmation draws each origin as a request of its own, in a
    sentence that says the pages of that site may call the server.

### the list in Settings

15. `PUT /api/v1/allowed-origins` replaces the Settings list. It takes the
    admin role on the organization.
16. The list is kept in the machine-owned file. It is added to the list in
    the operator's file and never replaces it, because this server does not
    write that file.
17. An entry is held to what `core.api.allowed_origins` is held to. A
    wildcard is refused, and an `http` origin is accepted, as a page under
    development needs one.
18. Every source applies at once. The answer to a browser reads one set that
    is rebuilt on each change.

## enforcement

| rule | gate |
|---|---|
| 1, 2 | `sdk/manifest/origins_test.go` |
| 3, 5, 6, 7 | `internal/app/moduleorigins_test.go`, and `TestEveryAskSurvivesEveryHop` for the marketplace row, the index row and the API |
| 4 | `TestApprovingAnOriginNeedsTheOrganization` |
| 8, 9 | `internal/app/resource/origins_test.go` and `internal/app/origins_test.go` |
| 12, 15 | `internal/transport/http/httpapi/origins_test.go` |
| 13, 14 | the tests of the network section in `packages/svelte` and of the request list in `frontend` |
| 16, 17 | `internal/app/origins_test.go` |
| 18 | `TestAStoredOriginIsAnsweredWithNoRestart` |

## rejected

| option | why not |
|---|---|
| write the origin into `core.api.allowed_origins` at install | the server does not write the operator's file, and a copy elsewhere has to be found and removed at uninstall |
| copy the origin into the Settings list | after a removal nobody knows whether the person or the extension added it |
| an approval of its own, as a port has | a port can be moved and can collide with another. An origin is granted or not, like a permission |
| an `http` origin in a manifest | anybody on the network path can rewrite a page served over plain http, and a release asks every operator at once |
| a wildcard host such as `*.example` | the grant would cover hosts the operator never read |
| the Settings list replaces the file's list | an operator who edits the file would see no effect and no message |
| a restart to apply a change | the answer already depends on one set, and reading it for each request costs one map lookup |

## open items

- the marketplace has to state `allowed_origins` for a release. Until it does,
  an approved install of such a release from the marketplace is refused by
  rule 6. An extension offered from `core.modules_local` installs
- an entry carries no sentence saying why, as a `needs.listen` entry does
- Settings cannot take one origin away from an extension that stays switched
  on. The declaration can, and rule 7 keeps that choice
