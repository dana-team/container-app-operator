# GitHub Webhook Configuration Guide (OnCommit)

This guide explains how to configure and test the GitHub webhook feature (`OnCommit`) for the Container App Operator.

## Prerequisites

- The operator is deployed and running.
- The `OnCommit` feature is enabled in the operator configuration.
- You have a GitHub repository to use for testing.

## 1. Expose the Webhook Service

The operator's webhook server listens on `/webhooks/git`. To receive events from GitHub, you must expose this service to the internet.

### Using an Ingress (Example)

Create an Ingress that points to the `git-webhook-service` in the `system` namespace on port `443`.

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: webhook-ingress
  namespace: system
spec:
  rules:
  - host: webhook.your-domain.com
    http:
      paths:
      - path: /webhooks/git
        pathType: Exact
        backend:
          service:
            name: git-webhook-service
            port:
              number: 443
```

## 2. Create a Webhook Secret

GitHub uses a shared secret to sign payloads. Create a Kubernetes Secret in the same namespace where your `CappBuild` will reside:

```bash
kubectl create secret generic github-webhook-secret \
  --from-literal=token=REPLACE_WITH_YOUR_SHARED_SECRET \
  -n your-namespace
```

## 3. Configure the CappBuild

Update your `CappBuild` to enable `OnCommit` mode and reference the secret created above.

```yaml
apiVersion: rcs.dana.io/v1alpha1
kind: CappBuild
metadata:
  name: my-app-build
  namespace: your-namespace
spec:
  source:
    type: Git
    git:
      url: https://github.com/your-user/your-repo
      revision: main # The webhook handler filters by this branch
  rebuild:
    mode: OnCommit # Enable OnCommit mode
  onCommit:
    webhookSecretRef:
      name: github-webhook-secret
      key: token
  # ... other fields (output, buildFile)
```

## 4. Configure GitHub Webhook

1. Go to your GitHub repository: **Settings > Webhooks > Add webhook**.
2. **Payload URL**: `https://<your-exposed-domain>/webhooks/git`
3. **Content type**: `application/json`
4. **Secret**: The same secret value you used in Step 2 (`REPLACE_WITH_YOUR_SHARED_SECRET`).
5. **Which events**: Select **Just the push event**.

## 5. Testing the Feature

1. **Push a change** to the branch specified in your `CappBuild` (e.g., `main`).
2. **Check the Operator Logs**:
   ```bash
   kubectl logs -n system -l control-plane=controller-manager
   ```
   Look for `git-webhook` logs indicating the event was received and accepted.
3. **Verify CappBuild Status**:
   ```bash
   kubectl get cappbuild my-app-build -o yaml
   ```
   You should see the `status.onCommit.lastReceived` field updated with the latest commit SHA and the `status.onCommit.pending` field populated, which triggers a new `BuildRun`.
