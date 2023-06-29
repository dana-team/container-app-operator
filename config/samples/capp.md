## Capp spec

```
apiVersion: rcs.dana.io/v1alpha1
kind: Capp
metadata:
  name: capp-example
spec:
  configurationSpec:
    template:
      metadata:
        creationTimestamp: null
      spec:
        volumes:
        - name: nfs-share
          nfs:
            server: nfs.server.example.tld
            path: /nfs-share
        containers:
        - env:
          - name: ENV
            value: prep
          - name: APP
            value: my-application
          envFrom:
          - secretRef:
            name: test-secret
          image: example-python-app:v1
          name: my-application
          imagePullSecrets:
          - name: regcred
          volumeMounts:
          - name: nfs-share
            mountPath: /mnt/nfs-share
          ports:
          - containerPort: 9000
          readinessProbe:
            exec:
              command:
              - cat
              - /tmp/healthy
              initialDelaySeconds: 5
              periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /healthz
              port: 9000
                httpHeaders:
                - name: Custom-Header
                  value: Awesome
            initialDelaySeconds: 3
            periodSeconds: 3
  routeSpec:
    hostname: example.com
    https: true
    secretName: secret-tls
  scaleMetric: rps
  site: ocp-nikola
```

1. `name:` The name of the Capp resource, in this case, it is capp-example.
2. `volumes:` The list of volumes that the container will mount, in this case, an NFS share.

    a. `name:` The name of the volume that the container will mount, in this case, nfs-share.

3. `env:` The list of environment variables that the container will use, including the values for ENV and APP.

    a. `envFrom:` The list of environment variables that the container will use, sourced from a Kubernetes secret named test-secret.

4. `imagePullSecrets:` secret to pull image from private registry.
5. `image:` The name of the container image that the pod will use, in this case, example-python-app:v1.

    a. `name:` The name of the container that the pod will create, in this case, my-application.

    b. `volumeMounts:` The list of volumes that the container will mount, in this case, the nfs-share volume.

    c. `ports:` The list of ports that the container will listen on, in this case, a single port on 9000.

    d. `livenessProbe:` The specification for the liveness probe that will be used to determine if the container is healthy.

    e. `readinessProbe:` The specification for the readiness probe that will be used to determine if the container is ready and getting requests.

6. `routeSpec:` The specification for the capp routing.

    a. `hostname:` Custom DNS name for the capp route. 

    b.`https:` Enable TLS for the route.

    c. `secretName:` Secret name containes TLS cerifacte.

7. `site:` Where to deploy the Capp, it can be a specific cluster or a placement name. 

## Capp status

```
 status:
    Revisions:
    - RevisionsStatus:
        conditions:
        - lastTransitionTime: "2023-04-16T14:32:54Z"
          status: Unknown
          type: ContainerHealthy
        - lastTransitionTime: "2023-04-16T14:32:54Z"
          reason: ResolvingDigests
          status: Unknown
          type: Ready
        - lastTransitionTime: "2023-04-16T14:32:54Z"
          reason: ResolvingDigests
          status: Unknown
          type: ResourcesAvailable
        observedGeneration: 1
      name: koki-00001
    - RevisionsStatus:
        conditions:
        - lastTransitionTime: "2023-04-16T14:32:54Z"
          status: Unknown
          type: ContainerHealthy
        - lastTransitionTime: "2023-04-16T14:32:54Z"
          reason: ResolvingDigests
          status: Unknown
          type: Ready
        - lastTransitionTime: "2023-04-16T14:32:54Z"
          reason: ResolvingDigests
          status: Unknown
          type: ResourcesAvailable
        observedGeneration: 1
      name: koki-00002
    applicationLinks:
      clusterSegment: 10.129.0.0/23
      consoleLink: console-openshift-console.apps.ocp-nikola.os-pub.com
      site: ocp-nikola
    knativeObjectStatus:
      conditions:
      - lastTransitionTime: "2023-04-16T14:32:54Z"
        status: Unknown
        type: ConfigurationsReady
      - lastTransitionTime: "2023-04-16T14:32:54Z"
        message: Configuration "koki" is waiting for a Revision to become ready.
        reason: RevisionMissing
        status: Unknown
        type: Ready
      - lastTransitionTime: "2023-04-16T14:32:54Z"
        message: Configuration "koki" is waiting for a Revision to become ready.
        reason: RevisionMissing
        status: Unknown
        type: RoutesReady
      latestCreatedRevisionName: koki-00002
      observedGeneration: 2
      url: https://koki-default.apps.ocp-nikola.os-pub.com
```

## Capp tls certifacte

```
apiVersion: v1
kind: Secret
metadata:
  name: secret-tls
type: kubernetes.io/tls
data:
  # the data is abbreviated in this example
  tls.crt: |
        MIIC2DCCAcCgAwIBAgIBATANBgkqh ...
  tls.key: |
        MIIEpgIBAAKCAQEA7yn3bRHQ5FHMQ ..
```

## Capp private pull registry secret
```
apiVersion: v1
kind: Secret
metadata:
  name: myregistrykey
  namespace: awesomeapps
data:
  .dockerconfigjson: UmVhbGx5IHJlYWxseSByZWVlZWVlZWVlZWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWxsbGxsbGxsbGxsbGxsbGxsbGxsbGxsbGxsbGxsbGx5eXl5eXl5eXl5eXl5eXl5eXl5eSBsbGxsbGxsbGxsbGxsbG9vb29vb29vb29vb29vb29vb29vb29vb29vb25ubm5ubm5ubm5ubm5ubm5ubm5ubm5ubmdnZ2dnZ2dnZ2dnZ2dnZ2dnZ2cgYXV0aCBrZXlzCg==
type: kubernetes.io/dockerconfigjson
```
