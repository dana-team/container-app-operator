# Gateway API routing for Capp hostnames

This document explains why the operator can route Capp hostnames through Gateway API objects it creates itself,
how that works, and every option that was considered on the way. It is written in the order things happened, so
the reasons for each decision are visible.

- [Summary](#summary)
- [How a Capp hostname was served before](#how-a-capp-hostname-was-served-before)
- [Order of events](#order-of-events)
- [Options considered](#options-considered)
- [Design](#design)
- [Configuration](#configuration)
- [Platform prerequisites](#platform-prerequisites)
- [Migrating a cluster](#migrating-a-cluster)
- [Limitations and open questions](#limitations-and-open-questions)

## Summary

With Knative on the `net-gateway-api` ingress (for example Envoy Gateway), two things break for Capps with a
`routeSpec.hostname`:

1. **No traffic reaches a hostname Capp.** The hostname is served through a Knative `DomainMapping`, and Knative
   routes a DomainMapping to the Knative Service's own Service, which is `type: ExternalName`. Envoy Gateway refuses
   ExternalName backends, so the request ends with a 500 before Knative's activator sees it, and no pod starts.
   This affects every Capp with a hostname, with or without TLS.
2. **At most about 60 TLS Capps per cluster.** `net-gateway-api` adds one listener per TLS hostname to the single
   external Gateway. The Gateway API CRD caps `Gateway.spec.listeners` at 64.

When `CappConfig.spec.gatewayAPI` is set, the operator no longer creates a DomainMapping. For each Capp with a
hostname it creates:

| Object | Where | Purpose |
|---|---|---|
| `ClusterDomainClaim` (Knative) | cluster scoped, named after the hostname | Keeps the hostname unique in the cluster, as a DomainMapping does |
| `HTTPRoute` `capp-<hostname>` | Capp namespace | Hostname → Knative's local gateway Service, Host rewritten to `<capp>.<ns>.svc.<cluster-domain>` |
| `ReferenceGrant` `capp-routes.<ns>` | local gateway Service namespace | Lets the HTTPRoutes of the Capp's namespace reference the Service across namespaces; one per namespace, shared by its Capps |
| `Certificate` (cert-manager) | Capp namespace | Unchanged; only with `tlsEnabled` in `ListenerSet` mode |
| `ListenerSet` `capp-<hostname>` | Capp namespace | One HTTPS listener with the Capp's own certificate, attached to the Gateway; only with `tlsEnabled` in `ListenerSet` mode |
| `CNAMERecord` (Crossplane) | Capp namespace | Unchanged |

Traffic splits, rollouts, the activator and scale from zero stay with Knative. Without `gatewayAPI` the operator
behaves exactly as before.

## How a Capp hostname was served before

```
Capp ──▶ Knative Service ──▶ Route ──▶ KIngress (default URL)
   │
   ├──▶ Certificate (cert-manager) ──▶ Secret <hostname>-tls
   ├──▶ DomainMapping <hostname> (spec.tls.secretName once the Secret exists)
   │        └──▶ KIngress ──▶ ingress (Kourier, or net-gateway-api)
   └──▶ CNAMERecord <hostname> ──▶ CappConfig.spec.dnsConfig.cname
```

Knative turns a DomainMapping into a KIngress with one rule: hostname → the Knative Service's own Service on port
80, with the Host header rewritten to `<ksvc>.<ns>.svc.cluster.local` and a `K-Original-Host` header added. That
Service is an ExternalName Service that points at Knative's local (cluster-internal) gateway. The local gateway
holds the Knative Service's internal route, which carries the revision weights. So a DomainMapping request takes
two hops: external gateway, then local gateway, then the revisions. Kourier follows the ExternalName without
complaint.

## Order of events

The work started as a migration of Knative Services from Kourier to Envoy Gateway on an air-gapped OpenShift 4.22
cluster, with the upstream Knative Operator 1.24 and Envoy Gateway v1.10.0. Most early problems were installation
problems, listed here because they explain the final setup.

1. **Gateway API CRDs.** On OpenShift 4.19+ the Gateway API CRDs belong to the ingress operator and are protected
   by a ValidatingAdmissionPolicy, so only Envoy Gateway's own CRDs are installed, with the Gateway API CRDs left
   out of the Envoy Gateway chart.
2. **Mirrored images.** Envoy Gateway refers to its images by tag, so an ImageDigestMirrorSet does not apply; the
   images had to be set explicitly (controller, shutdown manager, and `EnvoyProxy` container image).
3. **No HTTPRoute was created for a Knative Service.** The KnativeServing `spec.registry.override` used bare
   `controller` and `webhook` keys, which also matched the net-gateway-api Deployments' containers and replaced
   their images with Serving's. Keys of the form `<deployment>/<container>` fixed it.
4. **`endpoints knative-internal not found`.** `config-gateway` named the gateway Services in the wrong namespace.
   Without Envoy Gateway's GatewayNamespace mode, the proxy Services live in `envoy-gateway-system`.
5. **Default URL unreachable.** The `LoadBalancer` Service of the external Envoy stayed pending. MetalLB (or a
   cloud load balancer) is needed for an external IP.
6. **HTTPS for Capps.** Capps with `tlsEnabled` keep their cert-manager Certificate; `net-gateway-api` turns the
   DomainMapping's `spec.tls` into a `kni-<uid>` listener on the external Gateway plus a ReferenceGrant for the
   Secret. Nothing in the Capp operator seemed to need changing.
7. **The 64-listener limit.** One listener per TLS Capp on one Gateway object, with a CRD cap of 64. Issue
   [#771](https://github.com/dana-team/container-app-operator/issues/771) was opened with a first proposal: a
   wildcard listener on the Gateway and no per-Capp certificate.
8. **Sharing one load balancer between Gateways.** Envoy Gateway's merged gateways let several Gateways share one
   Envoy and one Service. This does not help while Knative writes the listeners: `net-gateway-api` 1.24 refuses
   more than one external gateway in `config-gateway` (`only a single external gateway is supported`).
9. **No traffic for a TLS Capp, and no pods.** The DomainMapping's HTTPRoute had
   `ResolvedRefs=False`: *service is of type ExternalName which is not supported as a backend, use an Envoy Gateway
   Backend resource with an FQDN endpoint instead*. `net-gateway-api` writes every backend as `kind: Service`, so
   Knative cannot be configured around this. The issue affects every Capp with a hostname, not only TLS ones.
10. **Looking for a way without operator-created Gateway API objects.** See
    [Options considered](#options-considered). The constraint that decided it: several clusters share the same DNS
    zone, so no wildcard DNS record can point at one cluster's load balancer, and per-Capp DNS records must stay.
11. **Decision.** The operator replaces the DomainMapping with its own HTTPRoute, keeps every existing feature, and
    serves TLS through one ListenerSet per Capp. This document and the change that adds it are the result.
12. **No ListenerSet on the cluster.** The Gateway API version OpenShift 4.22 ships does not serve `ListenerSet`, and
    OpenShift does not allow the Gateway API CRDs to be replaced. The operator therefore also supports
    `tlsMode: Gateway`, and the cluster uses it: one wildcard listener for `*.<zone>` on the external Gateway, with a
    wildcard certificate issued per cluster. Per-Capp CNAMEs are kept, so shared zones still work; per-Capp
    certificates are given up until ListenerSet is available. Sharded Gateways (below) stay the option if per-Capp
    certificates become a requirement. In `ListenerSet` mode on such a cluster the hostname is still served over HTTP,
    and `RouteReady` reports that HTTPS needs `tlsMode: Gateway`.
13. **Review.** A review of the first version found that the hostname claim could be lost or taken over: Knative's
    DomainMapping finalizer deletes the claim the operator had just adopted, two Capps of one namespace could relabel
    one claim back and forth, a Certificate (with its private key) was issued before the claim was checked, a lost
    claim left the old route serving, and turning `gatewayAPI` off deleted routes before the DomainMappings were
    ready. It also found a route rewritten on every reconcile (the CRD's `weight` default), a claim read from a cache
    that did not have it yet, Secret handling that could delete a user's Secret or leave a reissued one behind, and
    one ReferenceGrant per Capp. [Design](#design) describes the behaviour after those fixes. Looking up the Gateway
    API kinds on every reconcile, instead of once at startup, was considered and not done: the watches need a restart
    anyway, and a lookup of a missing kind costs a discovery request on clusters without Gateway API.

## Options considered

The requirement at the end was to keep every existing feature: custom hostname, per-Capp certificate from the
CappConfig issuer, per-Capp DNS record, the default Knative URL, hostname uniqueness, the hostname pattern check,
and more than 64 TLS Capps per cluster.

| # | Option | Fixes ExternalName | Lifts 64-listener cap | Keeps every feature | Why not chosen |
|---|---|---|---|---|---|
| 1 | Knative's own certificates (`external-domain-tls`, `config-certmanager`) | No | No | — | Still a DomainMapping per hostname, and still one `kni-*` listener per host |
| 2 | Wildcard listener on the Gateway, Capps without per-Capp TLS (first version of #771) | No | Yes | No | Hostnames still go through DomainMappings; per-Capp certificates and dotted hostnames lost |
| 3 | `domain-template` reading a Capp annotation, so the Knative Service itself has the custom hostname | Yes | With a wildcard listener | No | The Capp loses its default URL; per-Capp certificates replaced by a wildcard; uniqueness check lost. Possible with no operator change only with wildcard DNS, which shared zones rule out |
| 4 | ReferenceGrant | No | No | — | A grant only permits cross-namespace references; the ExternalName backend is in the Capp's own namespace and is refused for its type |
| 5 | Envoy Gateway `Backend` with an FQDN endpoint | Only in self-written routes | No | — | `net-gateway-api` never writes a `Backend` reference; using one needs operator-written routes anyway, plus `extensionApis.enableBackend`, which lets anyone allowed to create Backends point Envoy at any address |
| 6 | Several Gateways merged onto one load balancer, filled by Knative | No | No | — | `net-gateway-api` 1.24 supports a single external gateway |
| 7 | Upstream fix in `net-gateway-api` (follow an ExternalName that points at the local gateway) | Yes | No | Not at scale | Depends on Knative's release schedule, and the listener cap remains until Knative supports ListenerSets |
| 8 | Keep Kourier for hostname Capps (two ingresses, per-Service `networking.knative.dev/ingress.class`) | Avoids it | Avoids it | Yes | A migration bridge, not an end state: two ingress stacks and load balancers, and one `dnsConfig.cname` |
| 9 | **Operator-managed routing** (this change) | **Yes** | **Yes** | **Yes** | Chosen |

### Within option 9: how to serve TLS

| Choice | Notes |
|---|---|
| **ListenerSet per Capp (chosen)** | One small object per Capp, owned by the Capp, garbage collected with it. The certificate Secret is in the ListenerSet's namespace, so no ReferenceGrant is needed for it. No shared object is written per Capp. `ListenerSet` is part of the standard channel in Gateway API v1.6, which this operator builds against. |
| Shards of standard Gateways, merged onto one Envoy with `mergeGateways` | Works with older Gateway API versions, but the operator would add and remove listeners in shared Gateway objects: capacity allocation, update conflicts between Capps, creating shards, and cleanup with finalizers. Kept as the fallback if ListenerSets are not available. |
| `tlsMode: Gateway` (also implemented) | No per-Capp certificate or listener: a listener of the Gateway itself (for example `*.<zone>`) serves HTTPS. For clusters without ListenerSet support, at the cost of per-Capp certificates. |

### Within option 9: one hop or two

| Choice | Notes |
|---|---|
| **Two hops through the local gateway (chosen)** | The route sends the request to Knative's local gateway Service with the Host rewritten to the Knative Service's cluster-local name. This is exactly what a DomainMapping does today, also on Kourier. It depends only on Knative's documented cluster-local host name. |
| One hop: copy the rules of Knative's own HTTPRoute for the Knative Service and change only `hostnames` | Saves a hop, but ties the operator to how `net-gateway-api` names, labels and builds its routes. Every rollout weight change would reach the hostname one operator reconcile later, and a change to the route format in a Knative upgrade could break it. A later improvement if the second hop's latency or load matters. |
| One hop: build the weights from the Knative Service status | Would have to re-create Knative's routing headers and activator handling exactly. |

An HTTPRoute cannot forward to another HTTPRoute, and Envoy does not re-match routes after a Host rewrite, so
"forward to the Knative Service's route" always means a second Envoy hop.

## Design

### Objects and request path

```
client ──TLS (ListenerSet listener, Capp certificate)──▶ external Envoy (Gateway from gatewayAPI.gateway)
          HTTPRoute capp-<hostname>:  Host → <capp>.<ns>.svc.cluster.local,  K-Original-Host: <hostname>
       ──▶ Service gatewayAPI.localGatewayService (ReferenceGrant capp-routes.<ns>)
       ──▶ local Envoy ──▶ Knative's internal route for the Knative Service ──▶ activator / revisions
```

- The HTTPRoute always attaches to the Gateway, so the hostname is served over HTTP as soon as the claim exists. It
  also attaches to the Capp's ListenerSet once that exists.
- The ListenerSet is created only after cert-manager has issued the Secret, like the DomainMapping's `spec.tls`
  today. The Certificate's `secretTemplate` carries the Capp labels, and the operator watches those Secrets, so the
  Secret's creation triggers the next reconcile even when it reaches the cache after the Certificate's Ready event.
- The route sets the backend `weight` to 1, the value the CRD defaults it to, so an unchanged route is never
  rewritten.
- The route's request timeout is `0s` (no timeout on the outer hop), as `net-gateway-api` sets on its own routes. The
  Capp's `routeTimeoutSeconds` is applied by Knative's internal route.

### Hostname uniqueness

Knative's DomainMapping reconciler claims a hostname for a namespace with a `ClusterDomainClaim` and refuses
DomainMappings in other namespaces. The operator creates the same claim itself:

- A claim of the same namespace without Capp labels is adopted and labelled, for example the one Knative created for
  the Capp's former DomainMapping. So is a claim labelled for a Capp that no longer exists.
- A claim labelled for another Capp of the same namespace that still exists is refused, so two Capps with one
  hostname cannot take the claim from each other, and deleting one cannot release the other's hostname.
- A claim of another namespace, or of another Capp, stops the Capp: no Certificate, ListenerSet or HTTPRoute is
  created, any the Capp already had are deleted so they cannot shadow the holder's route, and `RouteReady` reports who
  holds the hostname. The Certificate, ListenerSet and HTTPRoute managers each check the claim before writing, so the
  issuer never signs a certificate for a hostname another namespace holds.
- The check reads the claim from the API server, not the cache, because the claim manager may have created it earlier
  in the same reconcile.
- Claims are cluster scoped, so they carry Capp name and namespace labels instead of an owner reference, and are
  deleted when the Capp is deleted. The operator watches labelled claims: when Knative's DomainMapping finalizer deletes
  the adopted claim during the switch to Gateway API routing, the Capp is reconciled and the claim created again. The
  hostname is unclaimed for that short time.
- A claim, route or ListenerSet of a former hostname of the Capp (after the DNS zone changed) is deleted.
- When `gatewayAPI` is removed, claims are left in place, because the DomainMappings that replace the routes use the
  same claims.

The webhook's DNS lookup (`isDomainNameTaken`) still checks uniqueness across clusters that share a zone.

### Reconcile order

ClusterDomainClaim → Certificate → ListenerSet → HTTPRoute → DomainMapping → CNAMERecord. The claim comes first so
that nothing is issued or routed for a hostname another namespace holds. The HTTPRoute comes before the
DomainMapping's cleanup, so when a cluster switches to Gateway API routing, the new route exists before the
DomainMapping is deleted. When `gatewayAPI` is removed, the HTTPRoute and ListenerSet are kept until the DomainMapping
that replaces them is Ready, so the hostname is not left without a route; the DomainMapping watch triggers that
reconcile. While both exist, the Gateway serves the older route.

### Names and ownership

- The HTTPRoute and the ListenerSet are named `capp-<hostname>`. `net-gateway-api` names the HTTPRoute of a
  DomainMapping after the hostname in the same namespace; with the same name, the operator's route and Knative's
  would be one object while a cluster switches from DomainMappings, and Knative's garbage collection would delete
  the operator's route with the DomainMapping. Names longer than 253 characters keep the end of the hostname and
  add a hash.
- Knative copies the Knative Service's labels, which include the Capp labels, onto the HTTPRoutes it creates. The
  operator therefore treats an HTTPRoute or ListenerSet as its own only when it has an owner reference to the Capp:
  cleanup never touches Knative's routes, and an existing object of the same name that the Capp does not own is
  reported as an error instead of being taken over.

### Cleanup and ownership

| Object | Owner reference | Deleted when |
|---|---|---|
| HTTPRoute, ListenerSet, Certificate | Capp | No longer required, or with the Capp (garbage collection) |
| ReferenceGrant | none (other namespace) | When no operator HTTPRoute of another Capp in the namespace remains, or when `localGatewayService` moves to another namespace |
| ClusterDomainClaim | none (cluster scoped) | With the Capp |
| TLS Secret | none (cert-manager leaves it) | When no Certificate needs it anymore, or with the Capp; only a Secret whose `cert-manager.io/certificate-name` annotation names the Capp's Certificate. On Capp deletion the Certificate is deleted first, so cert-manager does not issue the Secret again |

### Clusters without Gateway API

At startup the operator asks the API server whether HTTPRoute and ListenerSet are served. It watches them only
when they are, and sends no request for a missing kind, so it keeps working on Kourier clusters. Install the
Gateway API CRDs before the operator starts, or restart it afterwards. A restart is needed in any case to start the
watches, so the kinds are not looked up again on every reconcile.

### Status

`RouteReady` checks, in order: the CNAMERecord, the ClusterDomainClaim, then (TLS in ListenerSet mode) the
Certificate and the ListenerSet's `Accepted` and `Programmed` conditions, then the HTTPRoute's `Accepted` and
`ResolvedRefs` conditions for each parent. The deprecated `status.routeStatus.domainMappingObjectStatus` stays empty
in this mode.

## Configuration

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: CappConfig
metadata:
  name: capp-config
  namespace: container-app-operator-system
spec:
  dnsConfig:
    zone: apps.example.com.
    cname: knative-ingress.apps.example.com.   # an A record for the external Envoy's load balancer IP
    provider: dns-default
    issuerRef: {name: my-issuer, kind: ClusterIssuer, group: cert-manager.io}
  gatewayAPI:
    gateway:                 # the external-gateways entry of Knative's config-gateway
      name: eg-external
      namespace: eg-external
    localGatewayService:     # the service of the local-gateways entry of Knative's config-gateway
      name: knative-internal
      namespace: envoy-gateway-system
      port: 80
    tlsMode: ListenerSet     # or Gateway
    httpsPort: 443
```

The Helm chart takes the same block under `config.gatewayAPI`.

## Platform prerequisites

- **Gateway API with ListenerSet** (`gateway.networking.k8s.io/v1` `ListenerSet`), and a Gateway API
  implementation that supports it, for `tlsMode: ListenerSet`. Check with `kubectl api-resources | grep -i
  listenerset`. Without it, use `tlsMode: Gateway`.
- **The external Gateway allows ListenerSets** from Capp namespaces:

  ```yaml
  spec:
    allowedListeners:
      namespaces:
        from: All          # or Selector, to limit it to Capp namespaces
  ```

- **The external load balancer exposes `httpsPort`.** Envoy Gateway adds Service ports for the listeners it
  programs.
- **For `tlsMode: Gateway`**, an HTTPS listener on the Gateway that covers Capp hostnames, for example a wildcard
  listener for `*.<zone>` with its certificate in the Gateway's namespace. A wildcard covers one DNS label.
- **DNS:** `dnsConfig.cname` must resolve to the external Envoy's load balancer address. Per-Capp CNAMERecords stay,
  so clusters can share a zone.

## Migrating a cluster

1. Install and verify Envoy Gateway and `net-gateway-api`; check that a default Knative URL works.
2. Allow ListenerSets on the external Gateway.
3. Set `gatewayAPI` in the CappConfig. Every Capp is reconciled: claims, routes and ListenerSets are created, then
   DomainMappings are deleted. The TLS Secret is kept and reused by the ListenerSet.
4. Point `dnsConfig.cname` at the Envoy load balancer. Existing CNAMERecords follow on their next reconcile.
5. To go back, remove `gatewayAPI`: DomainMappings are created again, and each Capp's route, ListenerSet and (when
   no other Capp of the namespace needs it) the namespace's grant are deleted once its DomainMapping is Ready.

Every Certificate the operator manages gets the Capp labels in its `secretTemplate` with this change, in both
routing modes. cert-manager updates the labels of the existing Secrets; it does not reissue them.

## Limitations and open questions

- **Not verified on a cluster.** The change is covered by unit tests with a fake client. It has not been run against
  Envoy Gateway; in particular Envoy Gateway v1.10's ListenerSet support, and the Gateway API version shipped by
  OpenShift 4.22, need checking before `tlsMode: ListenerSet` is used.
- **A hostname without a Gateway listener for it is HTTP only.** In ListenerSet mode, HTTPS starts once the
  certificate is issued.
- **No HTTP-to-HTTPS redirect**, as with `net-gateway-api`'s own routes.
- **Tag URLs** stay on the default Knative domain, as with DomainMappings.
- **Second hop.** Every hostname request passes the local gateway, so `knative-internal` must be sized and made
  highly available for external traffic. See the one-hop option above.
- **Gateway API CRDs installed after the operator started** need an operator restart before Gateway API routing is
  enabled.
- **`tlsMode: Gateway` covers one DNS label.** A wildcard certificate for `*.<zone>` does not cover `a.b.<zone>`: such
  a hostname still gets a route, but clients reject the certificate. The webhook does not refuse these hostnames yet.
