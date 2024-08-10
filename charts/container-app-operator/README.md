# capp-operator

![Version: 0.0.0](https://img.shields.io/badge/Version-0.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: latest](https://img.shields.io/badge/AppVersion-latest-informational?style=flat-square)

A Helm chart for container-app-operator

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Node affinity rules for scheduling pods. Allows you to specify advanced node selection constraints. |
| autoscaleConfig | object | `{"concurrency":"10","cpu":"80","memory":"70","name":"autoscale-defaults","rps":"200"}` | Configuration for the ConfigMap used to set autoscaling defaults. |
| autoscaleConfig.concurrency | string | `"10"` | The default concurrency limit for autoscaling. |
| autoscaleConfig.cpu | string | `"80"` | The default CPU utilization percentage for autoscaling. |
| autoscaleConfig.memory | string | `"70"` | The default memory utilization percentage for autoscaling. |
| autoscaleConfig.name | string | `"autoscale-defaults"` | The name of the ConfigMap containing autoscale defaults. |
| autoscaleConfig.rps | string | `"200"` | The default Requests Per Second (RPS) threshold for autoscaling. |
| dnsConfig | object | `{"cname":"ingress.capp-zone.com.","name":"dns-config","zone":"capp-zone.com."}` | Configuration for the DNS. |
| dnsConfig.cname | string | `"ingress.capp-zone.com."` | The canonical name that CNAMEs created by the operator should point at. |
| dnsConfig.name | string | `"dns-config"` | The name of the DNS configMap. |
| dnsConfig.zone | string | `"capp-zone.com."` | The DNS zone for the application. |
| fullnameOverride | string | `""` |  |
| image.kubeRbacProxy.pullPolicy | string | `"IfNotPresent"` | The pull policy for the image. |
| image.kubeRbacProxy.repository | string | `"gcr.io/kubebuilder/kube-rbac-proxy"` | The repository of the kube-rbac-proxy container image. |
| image.kubeRbacProxy.tag | string | `"v0.14.1"` | The tag of the kube-rbac-proxy container image. |
| image.manager.pullPolicy | string | `"IfNotPresent"` | The pull policy for the image. |
| image.manager.repository | string | `"ghcr.io/dana-team/container-app-operator"` | The repository of the manager container image. |
| image.manager.tag | string | `""` | The tag of the manager container image. |
| klusterlet | object | `{"enabled":true,"namespace":"open-cluster-management-agent","serviceAccountName":"klusterlet-work-sa"}` | Configuration for the service account used by the Klusterlet work. |
| klusterlet.enabled | bool | `true` | Flag to indiciate whether to deploy Klusterlet-related resources (defaults to true) |
| klusterlet.namespace | string | `"open-cluster-management-agent"` | The namespace where the service account resides. |
| klusterlet.serviceAccountName | string | `"klusterlet-work-sa"` | The name of the Klusterset service account. |
| kubeRbacProxy | object | `{"args":["--secure-listen-address=0.0.0.0:8443","--upstream=http://127.0.0.1:8080/","--logtostderr=true","--v=0"],"ports":{"https":{"containerPort":8443,"name":"https","protocol":"TCP"}},"resources":{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"5m","memory":"64Mi"}},"securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}}` | Configuration for the kube-rbac-proxy container. |
| kubeRbacProxy.args | list | `["--secure-listen-address=0.0.0.0:8443","--upstream=http://127.0.0.1:8080/","--logtostderr=true","--v=0"]` | Command-line arguments passed to the kube-rbac-proxy container. |
| kubeRbacProxy.ports.https.containerPort | int | `8443` | The port for the HTTPS endpoint. |
| kubeRbacProxy.ports.https.name | string | `"https"` | The name of the HTTPS port. |
| kubeRbacProxy.ports.https.protocol | string | `"TCP"` | The protocol used by the HTTPS endpoint. |
| kubeRbacProxy.resources | object | `{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"5m","memory":"64Mi"}}` | Resource requests and limits for the kube-rbac-proxy container. |
| kubeRbacProxy.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}` | Security settings for the kube-rbac-proxy container. |
| livenessProbe | object | `{"initialDelaySeconds":15,"periodSeconds":20}` | Configuration for the liveness probe. |
| livenessProbe.initialDelaySeconds | int | `15` | The initial delay before the liveness probe is initiated. |
| livenessProbe.periodSeconds | int | `20` | The frequency (in seconds) with which the probe will be performed. |
| manager | object | `{"args":["--leader-elect","--health-probe-bind-address=:8081","--metrics-bind-address=127.0.0.1:8080"],"command":["/manager"],"ports":{"health":{"containerPort":8081,"name":"health","protocol":"TCP"}},"resources":{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"10m","memory":"64Mi"}},"securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}}` | Configuration for the manager container. |
| manager.args | list | `["--leader-elect","--health-probe-bind-address=:8081","--metrics-bind-address=127.0.0.1:8080"]` | Command-line arguments passed to the manager container. |
| manager.command | list | `["/manager"]` | Command-line commands passed to the manager container. |
| manager.ports.health.containerPort | int | `8081` | The port for the health check endpoint. |
| manager.ports.health.name | string | `"health"` | The name of the health check port. |
| manager.ports.health.protocol | string | `"TCP"` | The protocol used by the health check endpoint. |
| manager.resources | object | `{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"10m","memory":"64Mi"}}` | Resource requests and limits for the manager container. |
| manager.securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}}` | Security settings for the manager container. |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` | Node selector for scheduling pods. Allows you to specify node labels for pod assignment. |
| readinessProbe | object | `{"initialDelaySeconds":5,"periodSeconds":10}` | Configuration for the readiness probe. |
| readinessProbe.initialDelaySeconds | int | `5` | The initial delay before the readiness probe is initiated. |
| readinessProbe.periodSeconds | int | `10` | The frequency (in seconds) with which the probe will be performed. |
| replicaCount | int | `1` | The number of replicas for the deployment. |
| securityContext | object | `{}` | Pod-level security context for the entire pod. |
| service | object | `{"httpsPort":8443,"protocol":"TCP","targetPort":"https"}` | Configuration for the metrics service. |
| service.httpsPort | int | `8443` | The port for the HTTPS endpoint. |
| service.protocol | string | `"TCP"` | The protocol used by the HTTPS endpoint. |
| service.targetPort | string | `"https"` | The name of the target port. |
| tolerations | list | `[]` | Node tolerations for scheduling pods. Allows the pods to be scheduled on nodes with matching taints. |

