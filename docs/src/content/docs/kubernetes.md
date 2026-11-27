---
title: Kubernetes
description: The manifest set that deploys one tenant instance, what each object is for, how configuration and credentials reach the pod, what a rollout costs, and which parts have never been applied to a cluster.
audience: run
rewrite:
  describes: current
  implementedBy:
    - deploy/kubernetes
    - docker/server
    - internal/arch/kubernetes_test.go
    - internal/app/config/kubernetes_manifest_test.go
---

One instance is one tenant. The manifests deploy a single instance into a namespace of its own, so
one tenant's configuration, volumes and Service cannot be confused with another's.

```bash
kubectl create namespace acme
kubectl apply -k deploy/kubernetes/ -n acme
kubectl -n acme rollout status deployment/prusa-connect
```

The image is the one [Container Deployment](./container.md) describes, and
[Set the image](#set-the-image) says which reference to give the Deployment. Everything else below
is the cluster around it.

## Files and the objects they declare

| file | object | holds |
|---|---|---|
| `deploy/kubernetes/configmap.yaml` | ConfigMap | the tenant's `config.yaml` |
| `deploy/kubernetes/deployment.yaml` | Deployment | one pod, its probes and its security context |
| `deploy/kubernetes/service.yaml` | Service | ClusterIP on port 8080 |
| `deploy/kubernetes/pvc.yaml` | two PersistentVolumeClaims | the authorization graph, and the storage root |
| `deploy/kubernetes/kustomization.yaml` | Kustomization | the four above, so `apply -k` works |

## Volumes

| mount | from | why |
|---|---|---|
| `/etc/prusa-connect` | the ConfigMap, read-only | the file `-config` names |
| `/config/config.d` | claim `prusa-connect-state` | the authorization graph and the resource overlay |
| `/storage` | claim `prusa-connect-storage` | uploaded files |
| `/tmp` | an `emptyDir` | scratch a read-only root filesystem otherwise denies |

`/config/config.d` is the one that has to be there. The root filesystem is immutable, so without it
the core stops at startup with `mkdir config/config.d: read-only file system`.

Both claims are `ReadWriteOnce` and both are created by the default storage class. Name a
`storageClassName` on a cluster that has no default, and raise the storage claim for a tenant that
keeps a file library.

`fsGroup: 10001` is what makes the claims writable. A freshly provisioned volume arrives owned by
root, and the pod runs as uid 10001 with an empty capability set, so nothing else would let it
write.

## Configuration

The core reads one file and a few flags, and no environment variable. The ConfigMap is that file.

The Deployment mounts it as a whole directory rather than with `subPath`, and passes
`-config /etc/prusa-connect/config.yaml`. See
[Mounting the configuration file in Kubernetes](./container.md#mounting-the-configuration-file-in-kubernetes)
for the two mount modes that are wrong and what each one breaks.

**Editing the ConfigMap does not reach a running pod.** The file is read once at startup and nothing
re-reads it:

```bash
kubectl -n acme edit configmap prusa-connect-config
kubectl -n acme rollout restart deployment/prusa-connect
```

Three keys in the shipped ConfigMap are per-tenant and the rest are defaults:

| key | set it to |
|---|---|
| `core.site` | the tenant's name. Two nodes calling themselves the same thing publish events a federated parent cannot tell apart |
| `core.auth.issuer`, `core.auth.client_id`, `core.auth.redirect_uri` | the tenant's OpenID Connect registration |
| `core.principal` | the subject that holds the first grant |

While `core.auth.issuer` is empty nobody is authenticated and every request is served as
`core.principal`. Set an issuer before the instance is reachable by anyone you do not trust.

Credentials never go in this file. See
[Credentials arrive as mounted files](./container.md#credentials-arrive-as-mounted-files).

## Probes

| probe | path | period |
|---|---|---|
| `startupProbe` | `/api/v1/health` | 2s, 30 failures allowed |
| `livenessProbe` | `/api/v1/health` | 10s |
| `readinessProbe` | `/api/v1/readiness` | 5s |

They answer two different questions and must not be pointed at one path. Liveness says the process
is serving; a failure means restart it. Readiness says every instance the configuration declares
**required** is serving; a failure means the process is fine and is refusing traffic on purpose.
Restarting for that loses the diagnosis.

The shipped ConfigMap declares the `local` storage instance `required: true`. A claim that did not
bind then takes the pod out of the Service, rather than leaving it accepting uploads it cannot store.

`GET /api/v1/readiness` names the required instances that are not serving, in `blocked`. The image
carries no `curl` and no `wget`, so read it from your own machine:

```bash
kubectl -n acme port-forward deployment/prusa-connect 8080:8080
curl -s localhost:8080/api/v1/readiness
```

The binary answers the same question as an exit code, which is what the probes and the compose
health check use:

```bash
kubectl -n acme exec deployment/prusa-connect -- \
  /prusa_connect core -config /etc/prusa-connect/config.yaml healthcheck
```

## Publishing the Service

The Service is a ClusterIP, so what publishes it is an Ingress or a Gateway the deployment chooses.
Two things constrain that choice, and both come from the printers:

- a printer holds a websocket, so the proxy has to allow the upgrade and keep it open
- an idle event stream writes a comment frame every `core.events.heartbeat`, 15s by default. A proxy
  that closes an idle connection sooner than that drops readers who are behaving correctly

Printers dial the same listener as browsers unless
`core.connectors.<name>.config.listener.addr` sets a second socket. Setting
it means a second `containerPort`, a second Service port, and a second certificate. The printer
listener serves only `/p/register` and `/p/ws` and answers 404 to everything else, so it is what a
printer VLAN should reach.

A camera stream reaches a browser over WebRTC on UDP, which an Ingress does not
carry. Without UDP the live view falls back to a websocket through the Ingress,
which needs no setting. For WebRTC, set `core.webrtc.udp_port` and publish that
port as UDP on the same number, for example with a LoadBalancer Service. Name
the address a browser reaches it at in `core.webrtc.addresses`. The
[container page](./container.md#camera-streams) has an example.

## Rollouts

`strategy: Recreate` and `replicas: 1`, and both are forced rather than chosen. The costs are in
[Rollouts, replicas and the printer websocket](./container.md#rollouts-replicas-and-the-printer-websocket).

`terminationGracePeriodSeconds` is 30 and `core.shutdown_timeout` is 15s. The grace period has to
stay above the timeout, or the pod is killed part way through the drain it was asked to perform.
Raise both together.

## What is proved and what is not

**No cluster has run these manifests.** They are written from the image's measured behaviour, and
the parts that can be checked without a cluster are checked.

| claim | how it is held |
|---|---|
| the pod runs the core, not the shipping server | the first argument is compared against the word the command routes to the core |
| the ConfigMap is a configuration the server accepts | its `config.yaml` is run through the loader the server runs, with unknown keys refused |
| every write path has a writable volume under it | the three paths in the ConfigMap are compared against the mounts |
| every probe path answers 200 | the paths are compared against the ones a real container is proved to serve |
| the port agrees from `core.api.addr` to the Service | read through all three files |
| the uid matches the image | compared against the Dockerfile's build argument |
| the pod is hardened the way compose is | each setting paired with the compose setting that means the same |

These do not hold, and a first deployment is what would settle them:

| unproved | what would settle it |
|---|---|
| the manifests are accepted by an API server | apply them to a cluster |
| `fsGroup` makes both claims writable for uid 10001 | a write into each mount from the running pod |
| the probe timings suit a cold start on network storage | a first rollout, timed |
| a rolling update is what ends a printer session, and reconnection is what follows | a printer on a bench during a rollout |
| the resource requests and limits are the right size | a tenant under a real workload |

`kubectl apply --dry-run=client` is not evidence for any of the first list. It accepts a Deployment
carrying a field that does not exist, so it proves the YAML parses and the kind is recognised, and
nothing further.

## Set the image

`deploy/kubernetes/deployment.yaml` names `prusa-connect:local`, which is the tag the compose
build writes. Replace it with the published image at the release you deploy, on the container and
on the init container:

```yaml
image: ghcr.io/prusa3d/prusa-connect:0.2.0
```

The image is published for `linux/amd64` and `linux/arm64`, so a node of either kind pulls it with
no credential. `GET /api/v1/health` on the pod names the version.

A cluster that cannot reach `ghcr.io` takes an image you build, as
[Build the image from a release](./container.md#build-the-image-from-a-release) shows.
`PRUSA_CONNECT_TAG` in `.env` names the tag the build writes. Tag it for your registry, push it,
and name it in the Deployment. `registry.example.com` stands for yours:

```bash
docker tag prusa-connect:0.2.0 registry.example.com/prusa-connect:0.2.0
docker push registry.example.com/prusa-connect:0.2.0
```

## Troubleshooting

| Symptom | Cause |
|---|---|
| `mkdir config/config.d: read-only file system` | the state claim is not mounted, or `core.overlay` points outside it |
| the pod is `Running` and never `Ready` | a required instance is not serving. Read `blocked` in the readiness body |
| a key set in the ConfigMap has no effect | the pod has not restarted since the edit |
| the rollout hangs with two pods | the strategy is `RollingUpdate` and the claim is `ReadWriteOnce` |
| `permission denied` under `/storage` | `fsGroup` is missing, or the volume was provisioned before it was added |
| `ErrImagePull` or `ImagePullBackOff` | the Deployment still names `prusa-connect:local`. Name the published image, as [Set the image](#set-the-image) shows |
| `/api/v1/health` reports version `0.0.0-dev` | the image was built with no `PCL_BUILD_VERSION` |
