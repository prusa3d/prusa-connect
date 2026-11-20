---
title: "ADR-20261119-w4q: A credential this server minted also travels in X-Api-Key"
description: "The server reads a token it minted from X-Api-Key as well as from a bearer header, so a program that cannot send a bearer token, such as PrusaSlicer 2, signs in with an API credential."
date: 2026-11-19
status: proposed
supersedes: []
amends: []
tags:
  - adr
  - auth
---
# ADR-20261119-w4q: A credential this server minted also travels in X-Api-Key

<!-- nav:related -->
> [!NOTE]
> Index: [decisions](README.md) | Builds on [ADR-20261025-tpp](20261025-tpp-a-credential-carries-scopes-and-an-app-asks-a-person-for-one.md)

Some programs cannot send `Authorization: Bearer`. PrusaSlicer 2 sends a
print to a printer that runs PrusaLink. It sends the API key a person typed
in the `X-Api-Key` header, or a user name and a password with HTTP Digest. The server reads a credential only from a bearer header and from its
cookies, so such a program is refused before any route runs. This record lets
it send an API credential of this server in `X-Api-Key`.

## rules

1. The server reads a credential from `X-Api-Key` when the request carries
   no bearer header. It reads it only for text shaped like a credential this
   server minted: a token with the `pcl_` prefix. A token another issuer
   signed is read only as a bearer token.
2. A credential in `X-Api-Key` is the same credential with the same scopes
   and the same checks as in a bearer header. A bearer header wins when a
   request carries both, and both win over a cookie.
3. `X-Api-Key` is not in the headers the server allows for a cross-origin
   request. A page on another site cannot make a browser send it.
4. Opening a page session still takes the bearer header. A page session is
   for an app that collected its credential through the app pairing, and
   such an app sends a bearer token.

## enforcement

| rule | gate |
|---|---|
| 1, 2 | `TestTokenFromReadsEveryCarrier` in `internal/platform/auth/session` |
| 1, 2 | `TestAnAPIKeyResolvesAsTheSameCredentialAsItsBearerToken` in `internal/platform/auth/session` |

## rejected

| option | why not |
|---|---|
| HTTP Digest with a user name and a password | the server would keep a password for each program, and a token already says who is asking and what it may do |
| any value of `X-Api-Key`, whatever its shape | a token another issuer signed would gain a second way in that its issuer never offered |
| a module that reads the key itself on a public route | a module holds no secret it could check a token with, and a public route takes no staged body (ADR-20261112-2eb rule 7) |
