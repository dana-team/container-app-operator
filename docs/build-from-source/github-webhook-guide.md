# GitHub Webhook Configuration Guide (OnCommit)

This guide explains how to configure and test the GitHub webhook feature (`OnCommit`) for the Container App Operator by simulating a GitHub push event using `curl`.

## Prerequisites

- The operator is deployed via Helm with `cappBuild.onCommit.enabled=true`.
- You have a GitHub repository URL to use for testing.

## 0. Enable OnCommit Webhook in the Operator

Deploy or upgrade the operator with the OnCommit webhook enabled:

```bash
helm upgrade --install capp-operator charts/container-app-operator \
  --set cappBuild.enabled=true \
  --set cappBuild.onCommit.enabled=true \
  -n container-app-operator-system \
  --create-namespace
```

**Verify the webhook is registered:**

```bash
kubectl logs -n container-app-operator-system \
  -l app.kubernetes.io/name=container-app-operator --tail=50 | grep webhook
```

You should see: `"message":"git webhook handler registered at /webhooks/git"`

## 1. Expose the Webhook Service

The operator's webhook server listens on `/webhooks/git`. Use port forwarding to make it reachable on your local machine or VM:

```bash
kubectl port-forward -n container-app-operator-system svc/capp-operator-container-app-operator-git-webhook-service 8443:443 --address 0.0.0.0
```

Keep this command running in a separate terminal.

## 2. Create the Kubernetes Secret

Decide on a shared secret (e.g., `mysecret`). Create a Kubernetes Secret in the same namespace where your `CappBuild` will reside:

```bash
kubectl create secret generic github-webhook-secret \
  --from-literal=token=mysecret \
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

## 4. Simulate a GitHub Push Event

To test the feature without exposing your cluster to the public internet, you can simulate a GitHub push event using `curl`.

### Create the Payload File

Create a file named `payload.json` (ensure the URL and ref match your `CappBuild`):

```bash
cat > payload.json << 'EOF'
{
  "ref": "refs/heads/main",
  "after": "1234567890abcdef1234567890abcdef12345678",
  "repository": {
    "clone_url": "https://github.com/your-user/your-repo"
  }
}
EOF
```

### Send the Webhook Request

Since the operator validates the GitHub signature, you must sign the payload using the same secret (`mysecret`) used in Step 2.

```bash
SECRET="mysecret"
PAYLOAD_FILE="payload.json"
URL="https://localhost:8443/webhooks/git"

# Calculate the HMAC-SHA256 signature
SIGNATURE=$(openssl dgst -sha256 -hmac "$SECRET" "$PAYLOAD_FILE" | cut -d' ' -f2)

# Send the request
curl -k -X POST "$URL" \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: push" \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  --data-binary "@$PAYLOAD_FILE"
```

**Expected response:** HTTP 202 (silent success).

**Testing with different commits:** To trigger additional builds, change the `after` field to a new commit SHA and resend.

## 5. Verify the Results

1. **Check the Operator Logs**:
   ```bash
   kubectl logs -n container-app-operator-system -l app.kubernetes.io/name=container-app-operator --tail=20 | grep webhook
   ```
   You should see: `"git webhook accepted for <namespace>/<cappbuild-name>"`

2. **Verify BuildRun was Created**:
   ```bash
   kubectl get buildruns -n your-namespace -l rcs.dana.io/build-trigger=oncommit
   ```
   You should see a new BuildRun with `-oncommit-` in its name.

3. **Check CappBuild Status**:
   ```bash
   kubectl get cappbuild my-app-build -n your-namespace -o yaml
   ```
   The `status.onCommit.lastReceived` and `status.onCommit.pending` fields should be populated with the commit SHA from your payload.
