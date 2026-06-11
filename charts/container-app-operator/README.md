# container-app-operator

![Version: 0.1.1](https://img.shields.io/badge/Version-0.1.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

A Helm chart for Kubernetes

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| config | object | `{"allowedHostnamePatterns":[{"explanation":"any hostname","match":".*"}],"autoscaleConfig":{"activationScale":3,"concurrency":10,"cpu":80,"maxScaleDelay":600,"memory":70,"minReplicasLimit":10,"rps":200},"defaultResources":{"limits":{"cpu":"200m","memory":"200Mi"},"requests":{"cpu":"100m","memory":"100Mi"}},"dnsConfig":{"cname":"ingress.capp-zone.com.","issuerRef":{"group":"cert-manager.io","kind":"ClusterIssuer","name":"cert-issuer"},"provider":"dns-default","zone":"capp-zone.com."},"enabled":true,"maxKafkaConsumers":5,"revisionHistoryLimit":10}` | Configuration for CappConfig CRD |
| config.allowedHostnamePatterns[0] | object | `{"explanation":"any hostname","match":".*"}` | A list of hostname patterns that Capp workload hostnames must match. Each entry has a required `pattern` (regex) and an optional `explanation` shown in webhook error messages. |
| config.autoscaleConfig.activationScale | int | `3` | The default activation scale (minimum replicas before scaling starts). |
| config.autoscaleConfig.concurrency | int | `10` | The default concurrency limit for autoscaling. |
| config.autoscaleConfig.cpu | int | `80` | The default CPU utilization percentage for autoscaling. |
| config.autoscaleConfig.maxScaleDelay | int | `600` | The global maximum scale delay in seconds (maximum allowed value for scaleDelaySeconds). |
| config.autoscaleConfig.memory | int | `70` | The default memory utilization percentage for autoscaling. |
| config.autoscaleConfig.minReplicasLimit | int | `10` | The global minimum scale (maximum allowed value for minReplicas). |
| config.autoscaleConfig.rps | int | `200` | The default Requests Per Second (RPS) threshold for autoscaling. |
| config.defaultResources.limits | object | `{"cpu":"200m","memory":"200Mi"}` | Default compute resource limits applied to all Capp workloads. |
| config.defaultResources.limits.cpu | string | `"200m"` | Maximum requested CPU per Capp workload. |
| config.defaultResources.limits.memory | string | `"200Mi"` | Maximum allowed memory per Capp workload. |
| config.defaultResources.requests.cpu | string | `"100m"` | Default requested CPU per Capp workload. |
| config.defaultResources.requests.memory | string | `"100Mi"` | Default requested memory per Capp workload. |
| config.dnsConfig.cname | string | `"ingress.capp-zone.com."` | The canonical name that CNAMEs created by the operator should point at. |
| config.dnsConfig.issuerRef.group | string | `"cert-manager.io"` | The API group of the certificate issuer (e.g. cert-manager.io). |
| config.dnsConfig.issuerRef.kind | string | `"ClusterIssuer"` | The kind of the certificate issuer (e.g. ClusterIssuer). |
| config.dnsConfig.issuerRef.name | string | `"cert-issuer"` | The name of the certificate issuer. |
| config.dnsConfig.provider | string | `"dns-default"` | The name of the Crossplane DNS provider config. |
| config.dnsConfig.zone | string | `"capp-zone.com."` | The DNS zone for the application. |
| config.enabled | bool | `true` | Enable or disable creation of the CappConfig resource by Helm. |
| config.maxKafkaConsumers | int | `5` | The maximum allowed KafkaSource consumers per kafka source entry. |
| controllerManager.manager.args | list | `["--metrics-bind-address=:8443","--leader-elect"]` | Arguments passed to the controller manager container. |
| controllerManager.manager.containerSecurityContext.allowPrivilegeEscalation | bool | `false` | Whether a process can gain more privileges than its parent process. |
| controllerManager.manager.containerSecurityContext.capabilities | object | `{"drop":["ALL"]}` | Linux capabilities to drop from the container for improved security. |
| controllerManager.manager.image.imagePullPolicy | string | `"IfNotPresent"` | Controller manager container image pull policy. |
| controllerManager.manager.image.repository | string | `"ghcr.io/dana-team/container-app-operator"` | Controller manager container image repository. |
| controllerManager.manager.image.tag | string | `""` | Controller manager container image tag. |
| controllerManager.manager.resources.limits.cpu | string | `"500m"` | Maximum CPU limit for the controller manager container. |
| controllerManager.manager.resources.limits.memory | string | `"128Mi"` | Maximum memory limit for the controller manager container. |
| controllerManager.manager.resources.requests.cpu | string | `"10m"` | Minimum CPU request for the controller manager container. |
| controllerManager.manager.resources.requests.memory | string | `"64Mi"` | Minimum memory request for the controller manager container. |
| controllerManager.podSecurityContext.runAsNonRoot | bool | `true` | Run controller manager pods as non-root user. |
| controllerManager.replicas | int | `1` | Number of replicas for the controller manager Deployment. |
| controllerManager.serviceAccount.annotations | object | `{}` | Annotations to add to the service account used by the controller manager. |
| kubernetesClusterDomain | string | `"cluster.local"` | Domain name of the Kubernetes cluster. |
| metricsService | object | `{"annotations":{},"enabled":true,"port":8443,"type":"ClusterIP"}` | Metrics Service for the manager; `port` must match `--metrics-bind-address` in manager `args` (default 8443). |
| metricsService.annotations | object | `{}` | Optional annotations on the metrics Service. |
| metricsService.enabled | bool | `true` | If true, create a Service targeting the metrics port for scraping. |
| metricsService.type | string | `"ClusterIP"` | Service type for the metrics endpoint. |
| serviceMonitor | object | `{"enabled":false,"interval":"","labels":{},"metricRelabelings":[],"relabelings":[],"scrapeTimeout":"","tls":{"caFile":"","insecureSkipVerify":true}}` | kube-prometheus-stack / Prometheus Operator integration (optional). |
| serviceMonitor.enabled | bool | `false` | If true, create a ServiceMonitor (requires monitoring.coreos.com CRDs). |
| serviceMonitor.interval | string | `""` | Scrape interval (omit to use Prometheus default). |
| serviceMonitor.labels | object | `{}` | Extra labels on the ServiceMonitor (e.g. for prometheus operator selectors). |
| serviceMonitor.metricRelabelings | list | `[]` | Metric relabeling rules passed to the ServiceMonitor endpoint. |
| serviceMonitor.relabelings | list | `[]` | Relabeling rules passed to the ServiceMonitor endpoint. |
| serviceMonitor.scrapeTimeout | string | `""` | Scrape timeout (omit to use Prometheus default). |
| serviceMonitor.tls | object | `{"caFile":"","insecureSkipVerify":true}` | TLS options for HTTPS metrics scrape (default skips verify when the controller uses ephemeral certs). |
| serviceMonitor.tls.caFile | string | `""` | Path to PEM CA file on the Prometheus scraper pod; when set, typical pairing is insecureSkipVerify: false. |
| serviceMonitor.tls.insecureSkipVerify | bool | `true` | If false, set caFile to the CA bundle path on the Prometheus pod (e.g. OpenShift service CA). |
| webhookService.ports | list | `[{"port":443,"protocol":"TCP","targetPort":9443}]` | List of ports exposed by the webhook service. |
| webhookService.type | string | `"ClusterIP"` | Type of Kubernetes Service to expose the webhook (ClusterIP, NodePort, LoadBalancer). |

