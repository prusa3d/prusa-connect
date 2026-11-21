---
title: Use the API from a page
description: Install @prusa/connect, mint a token, list the printers, read the event stream and handle a refusal, in plain JavaScript and in Svelte.
audience: run
rewrite:
  describes: current
  implementedBy:
    - packages/typescript
    - config/config.yaml
    - scripts/check-client-sdk-install.sh
    - scripts/registry-publish.sh
    - docker/npm-registry
    - cmd/demo
---

You have a Prusa Connect running and want your own page to show the
printers. `@prusa/connect` is a typed client generated from the
server's own OpenAPI document, so every operation the server publishes is one
method with the server's own types. The event stream is an async iterable you
read with `for await`. The package has no runtime dependency. It needs only
`fetch`, in a browser or in Node 22 and later.

Every command below was run in the order it appears, and the output is what
it printed.

## Install the package

The package is not on npm yet. Two ways work today.

Pack it from this repository and add the tarball to your page:

```bash
cd packages/typescript && pnpm run build && pnpm pack
```

```
Tarball Details
prusa-connect-0.1.0.tgz
```

```bash
pnpm add ./prusa-connect-0.1.0.tgz
```

```
+ @prusa/connect 0.1.0
```

Or run a registry on your own machine and install from it, which is the next
section. A tarball is one file you have to pack again after every change; a
registry gives you `pnpm add` and a version number.

Once the package is on npm, `pnpm add @prusa/connect` with no `.npmrc`
replaces both.

## Publishing the packages to the local registry

The packages are not published to npmjs yet, because the repository is still
closed. Until it opens, a Verdaccio on your own machine holds them. It keeps
what you publish and is reached the same way npmjs is. Every project on the
machine installs from it with two lines in `.npmrc`. The release workflow
publishes to npmjs on the day the repository opens, and nothing in
your page changes then.

Four commands. The first starts the registry:

```bash
make registry-up
```

```
OK    the local npm registry is at http://127.0.0.1:54874/
```

The second builds the three client packages and publishes them. The version is the core's
`VERSION` with `-dev.<short sha>` after it, so every commit publishes a version
of its own and nothing collides with a release. `VERSION_TAG=0.2.0-alpha.1`
publishes under a version you choose instead:

```bash
make registry-publish
```

```
OK    @prusa/connect@0.1.0-dev.f9063f2d, @prusa/connect-ui@0.1.0-dev.f9063f2d and @prusa/frontend-standards@0.1.0-dev.f9063f2d are in http://127.0.0.1:54874/
```

The third writes the two `.npmrc` lines into your project. With no `DIR` it
prints them for you to paste:

```bash
make registry-use DIR=/path/to/your/page
```

```
wrote /path/to/your/page/.npmrc
wrote /path/to/your/page/pnpm-workspace.yaml

OK    /path/to/your/page installs the @prusa scope from http://127.0.0.1:54874/
      pnpm add @prusa/connect@dev @prusa/connect-ui@dev
```

Then install, in your project:

```bash
pnpm add @prusa/connect@dev @prusa/connect-ui@dev
```

```
dependencies:
+ @prusa/connect 0.1.0-dev.f9063f2d
+ @prusa/connect-ui 0.1.0-dev.f9063f2d
```

The fourth says what the registry holds:

```bash
make registry-status
```

```
@prusa/connect      0.1.0-dev.f9063f2d
                          latest -> 0.1.0-dev.f9063f2d, dev -> 0.1.0-dev.f9063f2d
@prusa/connect-ui   0.1.0-dev.f9063f2d
                          latest -> 0.1.0-dev.f9063f2d, dev -> 0.1.0-dev.f9063f2d
```

`make registry-down` stops the registry and keeps every version it holds, so
`make registry-up` the next morning finds them again. `make registry-reset`
drops the versions with it, which is what you want after publishing something
you would rather nobody installed.

Nothing here asks for a password. The registry accepts an unauthenticated
publish of the `@prusa` scope and listens on `127.0.0.1` alone, so it is
reachable from your machine and from nowhere else.

It is not the registry `make client-sdk-install-check` uses. That one is a
fixture on port 54873, started for one run and removed after it, and the two
run side by side without touching each other.

## Get a token

Every request below carries a token. `make demo` serves a login and, once
its printers are connected, prints the account to sign in with. The password
is new on every run:

```
Sign in as the demo account. Its password is new on every run:

  username  demo
  password  qsjphyacz2gosnkva22a
```

`GET /api/v1/auth/info` says which kind of server you have. This is the demo:

```json
{"provider":"local","credentials":true,"redirect":false,"needs_setup":false}
```

Log in, then mint. The demo prints the login line with the password filled
in, so you can copy it from its terminal:

```bash
curl -s -c cookies.txt -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H 'content-type: application/json' -d '{"username":"demo","password":"qsjphyacz2gosnkva22a"}'
curl -s -b cookies.txt -X POST http://127.0.0.1:8080/api/v1/auth/credentials \
  -H 'content-type: application/json' -d '{"name":"my page"}'
```

```json
{"expires_at":"2026-09-17T10:25:12.4642471+02:00"}
{"id":"iIpkmSntIjKIoJ4Kct46fC","name":"my page","subject":"user:operator",
 "created_at":"2026-09-10T10:25:12.5141504+02:00","expires_at":"2026-12-09T09:25:12.5141504+01:00",
 "token":"pcl_iIpkmSntIjKIoJ4Kct46fC_qmIpcdQgE58XKgp6QyJkDqzDqVfZ3WdSbLlx7Gm3Xlv1W8Aix"}
```

The `token` appears in this one answer. The server keeps a hash, and
`GET /api/v1/auth/credentials` lists the same credential with no token in it.
Keep it out of your source: hand it to the page after the person signs in,
and revoke it at `DELETE /api/v1/auth/credentials/{id}` when it leaks. A
request with no token is refused with `401 core.auth/TOKEN_INVALID`.

Only a session mints a token. A mint that presents a token instead is refused
with `403 core.auth/SESSION_REQUIRED`, so a leaked token cannot make a copy of
itself that outlives its revocation. A token lives ninety days unless the mint
names an `expires_at`, and that expiry is at most 365 days away.

Your own server mints the same way once it has accounts, which is
`core.auth.local: true` from [Local accounts](./local-accounts.md).

## Allow the page's origin

A browser sends a page's request only when the server names the page's
origin. A Vite dev server serves at `http://localhost:5173`, so the line in
`config/config.yaml` is:

```yaml
core:
  api:
    allowed_origins:
      - http://localhost:5173
```

`make demo` reads that file and keeps the origins it lists, so restart the
demo after adding one. A server reads the file once, when it starts.

An administrator can also add the origin in the web app, with no file and no
restart. Open **Settings**, then **Server**, then **Network**, and add it
under **Pages that may call this server**. The server answers it at once. That
list is added to the file's list. It also shows each origin an extension
brings while that extension is switched on. Without it, the console says this and the page shows
`NetworkError: request failed after 3 attempts: Failed to fetch`:

```
Access to fetch at 'http://127.0.0.1:8080/api/v1/devices?limit=100' from origin 'http://localhost:5173' has been blocked by CORS policy: Response to preflight request doesn't pass access control check: No 'Access-Control-Allow-Origin' header is present on the requested resource.
```

The match is exact. The same page served on port 4173 was refused the same
way. With the line in place, the demo answers the browser's preflight:

```console
$ curl -s -i -X OPTIONS http://127.0.0.1:8080/api/v1/devices -H 'Origin: http://localhost:5173' \
    -H 'Access-Control-Request-Method: GET' -H 'Access-Control-Request-Headers: authorization' | grep -i access-control
Access-Control-Allow-Credentials: true
Access-Control-Allow-Headers: Accept, Authorization, Content-Type, If-Match, If-Modified-Since, If-None-Match, Last-Event-ID, Traceparent
Access-Control-Allow-Methods: GET, HEAD, POST, PUT, PATCH, DELETE
Access-Control-Allow-Origin: http://localhost:5173
Access-Control-Max-Age: 600
```

## List the printers

`token` is the one minted above.

```js
import { PrusaConnect } from "@prusa/connect";

const local = new PrusaConnect({ url: "http://127.0.0.1:8080", auth: token });

const page = await local.listDevices({ query: { limit: 100 } });
console.log(page.total, "printers");
const { id, name, model, connected, state } = page.items[0];
console.log(JSON.stringify({ id, name, model, connected, state }));
```

```
4 printers
{"id":"abf6b4a1-9c0e-44cf-8766-118ab4e95337","name":"CoreOne DEMO-01","model":"CoreOne","connected":true,"state":{"/axis/x":0,"/axis/y":0,"/axis/z":10.200000000000001,"/flow":100,"/job/id":2,"/job/path":"/usb/tangle.bgcode","/job/progress":17,"/job/state":"PRINTING","/job/time_printing":51,"/job/time_remaining":249,"/speed":100,"/state":"PRINTING","/temperature/bed":60,"/temperature/bed_target":60,"/temperature/chamber":32,"/temperature/chamber_target":0,"/tools/0/active":true,"/tools/0/nozzle_temp":215,"/tools/0/nozzle_target_temp":215,"/tools/0/material_package_instance/package/material/abbreviation":"PLA"}}
```

A printer also carries `kind`, `vendor`, `firmware`, `serial`, `driver`,
`capabilities`, `actions`, `job`, `link`, `tags` and `issues`. `state` is
the live values by path, and `capabilities` is every command this printer
accepts.

## Subscribe to the event stream

```js
import { ApiError, PrusaConnect } from "@prusa/connect";

const local = new PrusaConnect({ url: "http://127.0.0.1:8080", auth: token });

const events = local.events({
  types: ["com.prusa.connect.device.record.changed", "com.prusa.connect.device.presence.changed"],
});

let seen = 0;
try {
  for await (const event of events) {
    console.log(event.type, event.id);
    if ((seen += 1) === 10) break;
  }
} catch (err) {
  if (err instanceof ApiError) console.log("the stream ended:", err.status, err.message);
  else throw err;
}
```

The demo's third printer leaves every minute and returns fifteen seconds
later, and each move is one presence event and one record event. So ten lines
take a few minutes:

```
com.prusa.connect.device.presence.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:8
com.prusa.connect.device.record.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:9
com.prusa.connect.device.presence.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:10
com.prusa.connect.device.record.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:11
com.prusa.connect.device.presence.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:12
com.prusa.connect.device.record.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:13
com.prusa.connect.device.presence.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:14
com.prusa.connect.device.record.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:15
com.prusa.connect.device.presence.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:16
com.prusa.connect.device.record.changed 01M256NYVQ1Z7ARX83S8YZA0MT:device/b7802b10-95f5-4aff-a5fc-b0489320865b:17
```

The `id` is the event's place in the log. When the connection drops, the
subscription waits, reconnects with that id, and the loop sees no gap. When
the server ends it, because the token was revoked or expired, the loop throws
an `ApiError`. Revoking the token above while the loop ran printed
`the stream ended: 401 the stream ended: the credential that opened it is no
longer accepted`.

## Handle a refusal

A new account holds no grant, so its token is refused wherever one is
needed, like administering accounts. `POST /api/v1/auth/accounts` as the
demo account creates one; log in as it and mint. With such a token:

```js
try {
  await local.listAccounts();
} catch (err) {
  if (!(err instanceof ApiError)) throw err;
  console.log(err.status, err.code, err.params);
  const codes = await local.getErrorCodeVocabulary();
  console.log(codes.items.find((c) => c.code === err.code)?.summary ?? err.message);
}
```

```
403 core.auth/ACCESS_DENIED { object: 'resource_kind:auth', action: 'configure_resource' }
The caller may not perform that action on that object
```

Branch on `err.code`, which comes from the registry on
[Error codes](./error-codes.md), and never on the wording of `err.message`.
The last line of the example is the one that turns a code into text for a
person. `err.traceId` is what an operator finds in the log.

## The same page in Svelte

This component was type-checked with `svelte-check`, built with Vite, and
served on port 5173 against the server that lists that origin. It is the
`svelte` consumer under `packages/typescript/testdata/consumers`, trimmed to
what this page showed.

```svelte
<script lang="ts">
  import { onMount } from "svelte";
  import { ApiError, PrusaConnect } from "@prusa/connect";

  let { url, auth }: { url: string; auth: string } = $props();

  let printers = $state<string[]>([]);
  let events = $state<string[]>([]);
  let refusal = $state<string | undefined>(undefined);

  onMount(() => {
    const local = new PrusaConnect({ url, auth });

    local
      .listDevices({ query: { limit: 100 } })
      .then((page) => {
        printers = (page.items ?? []).map((p) => `${p.name} ${p.state?.["/state"] ?? ""}`);
      })
      .catch((err: unknown) => {
        refusal = err instanceof ApiError ? `${err.code}: ${err.message}` : String(err);
      });

    const subscription = local.events({
      types: ["com.prusa.connect.device.record.changed", "com.prusa.connect.device.presence.changed"],
    });

    (async () => {
      try {
        for await (const event of subscription) {
          events = [...events, `${event.type} ${event.id ?? ""}`];
        }
      } catch (err) {
        refusal = err instanceof ApiError ? `${err.status}: ${err.message}` : String(err);
      }
    })();

    return () => subscription.close();
  });
</script>

<h1>Printers</h1>
{#if refusal !== undefined}<p>{refusal}</p>{/if}
<ul>
  {#each printers as printer (printer)}<li>{printer}</li>{/each}
</ul>
<h2>Events</h2>
<ol>
  {#each events as event, i (i)}<li>{event}</li>{/each}
</ol>
```

## From a page the core serves

The token above is for a page on another origin, or a script. A page the
core serves itself has no token to give: the person signed in through
`/api/v1/auth/login`, and the credential is the `pcl_session` cookie the
login set. That cookie is HttpOnly, so no script reads it, and the browser
attaches it to a request to the page's own origin on its own. Give the client
`{ kind: "session" }` and no `url`:

```js
import { PrusaConnect } from "@prusa/connect";

const local = new PrusaConnect({ auth: { kind: "session" } });

const page = await local.listDevices({ query: { limit: 100 } });
for await (const event of local.events()) {
  console.log(event.type, event.id);
}
```

Every request then goes to a relative path such as `/api/v1/devices`, with
no `Authorization` header and with `credentials: "same-origin"`, the event
stream included. No origin has to be allowed, because the page and the API
share one. A 401 means the session ended, so send the person back to the
login rather than reconnecting. The web app the core serves does this: it
shows the sign-in with the address in `?next=`, and after sign-in opens that
address if it is on the same origin. In Node there is no browser to hold the
cookie, so a script keeps the token above.

A person may open your page in many tabs. A browser opens at most six
connections to one host over plain HTTP, and each event stream keeps one of
them. With a stream per tab, six tabs take them all, and every other request
waits. Pass `shareEvents: true` and the tabs read one stream, which one tab
reads and passes to the others:

```js
const local = new PrusaConnect({ auth: { kind: "session" }, shareEvents: true });
```

Each `events()` call still gets the types and origins it asked for. When the
tab that reads closes, another tab opens the stream again from the last event
it heard.

## Read these next

| page | covers |
|---|---|
| `packages/typescript/README.md` | every method's request shape, `NetworkError`, and how the package is generated |
| [Event types](./event-types.md) | every `type` a subscription can ask for |
| [Error codes](./error-codes.md) | every `code` an `ApiError` can carry, and what to do about each |
| [Devices](./reference/api/devices.md) | every operation on a printer, with its client method, parameters, request and response types |
| [Local accounts](./local-accounts.md) | the login that mints the token above |
