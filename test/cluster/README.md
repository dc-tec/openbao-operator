Test Cluster Setup
==================

This directory contains helper manifests and configuration for running a local
end-to-end environment on k3d to exercise the OpenBao Operator, Gateway API,
Traefik, cert-manager, RustFS (S3-compatible storage for backups), and monitoring
(Prometheus and Grafana).

Prerequisites
-------------

- `k3d` (or another k3s-compatible environment)
- `kubectl`
- `kustomize` (or `kubectl` with `-k` support)
- `helm` (for Traefik, RustFS, and monitoring charts)

1. Create the k3d cluster
-------------------------

Create a k3d cluster using the provided config:

```sh
k3d cluster create --config test/cluster/cluster-config.yaml
```

This exposes:
- HTTP on `localhost:80`
- HTTPS on `localhost:443`
and configures a local registry at `k3d-registry.localhost:5000`.

2. Install Gateway API, Traefik RBAC, and cert-manager
------------------------------------------------------

Use the `kustomization.yaml` in this directory to install:
- Gateway API (standard install)
- Traefik Gateway RBAC
- cert-manager
- Local helper manifests (backend TLS policy, gateway certificate, admin SA, Traefik ServersTransport)

```sh
kubectl apply -k test/cluster
```

This will pull the following remote manifests:
- `https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.0/standard-install.yaml`
- `https://raw.githubusercontent.com/traefik/traefik/v3.6/docs/content/reference/dynamic-configuration/kubernetes-gateway-rbac.yml`
- `https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml`

and the local resources:
- `openbao-gateway-backend-tls-policy.yaml`
- `openbao-gateway-certificate.yaml`
- `openbao-admin-serviceaccount.yaml`
- `openbao-traefik-servers-transport.yaml`
- `monitoring-namespace.yaml` (creates the `monitoring` namespace)
- `openbao-operator-grafana-dashboard` ConfigMap (Grafana dashboard for operator metrics)

3. Install Prometheus and Grafana
-----------------------------------

Add the Prometheus Community Helm repo (if not already present):

```sh
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
```

Install the kube-prometheus-stack which includes Prometheus and Grafana:

```sh
helm upgrade --install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace \
  -f test/cluster/prometheus-values.yaml
```

This deploys:
- **Prometheus** for metrics collection and storage
- **Grafana** for visualization (default credentials: `admin`/`admin`)
- The OpenBao Operator dashboard is automatically discovered via the ConfigMap labeled with `grafana_dashboard: "1"`

Access Grafana:
```sh
kubectl port-forward -n monitoring svc/kube-prometheus-stack-grafana 3000:80
```

Then open http://localhost:3000 in your browser and log in with `admin`/`admin`.

The OpenBao Operator dashboard will be available under Dashboards → OpenBao Operator Metrics.

**Note:** To scrape metrics from the OpenBao Operator, ensure you have a ServiceMonitor configured.
The operator exposes metrics on the `/metrics` endpoint. See `config/prometheus/monitor.yaml` for an example.

**Troubleshooting Metrics Collection:**

If the Grafana dashboard shows no data, verify:

1. **Check if the metrics service exists:**
   ```sh
   kubectl get svc -n openbao-operator-system -l app.kubernetes.io/name=openbao-operator
   ```
   Should show: `openbao-operator-controller-manager-metrics-service`

2. **Check if the ServiceMonitor exists:**
   ```sh
   kubectl get servicemonitor -n openbao-operator-system
   ```

3. **Check Prometheus targets:**
   ```sh
   # Port-forward to Prometheus
   kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090
   # Then visit http://localhost:9090/targets and look for "openbao-operator" targets
   ```

4. **Check if metrics are being exposed:**
   ```sh
   kubectl port-forward -n openbao-operator-system svc/openbao-operator-controller-manager-metrics-service 8443:8443
   curl -k https://localhost:8443/metrics | grep openbao
   ```

5. **Check Prometheus logs for scraping errors:**
   ```sh
   kubectl logs -n monitoring -l app.kubernetes.io/name=prometheus --tail=50 | grep -i "openbao\|error"
   ```

4. Install Traefik with Gateway API support
-------------------------------------------

Add the Traefik Helm repo (if not already present):

```sh
helm repo add traefik https://traefik.github.io/charts
helm repo update
```

Install or upgrade Traefik using the Gateway-aware values:

```sh
helm upgrade --install traefik traefik/traefik \
  -n default --create-namespace \
  -f test/cluster/traefik-gateway-values.yaml
```

This creates a `Gateway` named `traefik-gateway` in the `default` namespace and
configures listeners on ports 80/443 with TLS termination using the `bao-tls`
certificate (from `openbao-gateway-certificate.yaml`).

5. Install RustFS for backups
-----------------------------

Add the RustFS Helm chart (from a local clone or chart registry) and install
using the test values. For example, if you have the `rustfs-helm` chart checked
out locally:

```sh
helm upgrade --install rustfs rustfs/rustfs \
  -n rustfs --create-namespace \
  -f test/cluster/rustfs-values.yaml
```

This deploys a RustFS cluster exposed via:
- In-cluster S3 endpoint: `http://rustfs-svc.rustfs.svc.cluster.local:9000`
- External HTTPS endpoint via Traefik: `https://rustfs.adfinis.localhost`

The default credentials (for backups) are:
- Access key: `rustfsadmin`
- Secret key: `rustfsadmin`

6. Deploy an OpenBaoCluster sample
----------------------------------

Apply one of the example OpenBaoCluster manifests that exercises:
- Self-initialization
- Gateway API
- Backups to RustFS

For example:

```sh
kubectl apply -f config/samples/openbao_v1alpha1_openbaocluster_full.yaml
```

This sample:
- Enables self-init and creates an `admin` policy with broad permissions.
- Uses Gateway API via the `traefik-gateway` Gateway and `bao-full.adfinis.localhost`.
- Schedules periodic backups to the RustFS bucket `openbao-backups`.

7. Admin access via Kubernetes auth
-----------------------------------

The `test/cluster/openbao-admin-serviceaccount.yaml` file creates a
`ServiceAccount` named `openbao-admin` in the `openbao-operator-system` namespace.
The `openbaocluster-full` sample configures a Kubernetes auth role that binds to
this service account, granting it the `admin` policy with broad permissions.

**Important:** Before authenticating, you must grant OpenBao permission to verify
Kubernetes service account tokens. OpenBao needs the `system:auth-delegator`
ClusterRole to call the Kubernetes TokenReview API.

After the OpenBaoCluster is deployed (step 5), create the ClusterRoleBinding:

```sh
kubectl apply -f test/cluster/openbao-kubernetes-auth-rbac.yaml
```

This grants the OpenBaoCluster's service account (`openbaocluster-full-serviceaccount`
in the `openbao-operator-system` namespace) permission to verify tokens. Wait for
the OpenBaoCluster to be fully initialized before proceeding.

To authenticate to OpenBao using this service account:

1. **Get a Kubernetes service account token:**

   ```sh
   # Create a TokenRequest to get a token for the service account
   kubectl create token openbao-admin -n openbao-operator-system --duration=24h
   ```

   This will output a JWT token that you can use for authentication.

2. **Authenticate to OpenBao using the Kubernetes auth method:**

   ```sh
   # Set the token as an environment variable
   K8S_TOKEN=$(kubectl create token openbao-admin -n openbao-operator-system --duration=24h)
   
   # Authenticate to OpenBao (replace with your cluster's hostname if different)
   curl -X POST https://bao-full.adfinis.localhost/v1/auth/kubernetes/login \
     -d "{\"role\": \"admin\", \"jwt\": \"$K8S_TOKEN\"}" \
     -k
   ```

   The response will contain an `auth.client_token` field with your OpenBao token.

3. **Use the OpenBao token for API calls:**

   ```sh
   # Extract the token from the login response (using jq if available)
   BAO_TOKEN=$(curl -s -X POST https://bao-full.adfinis.localhost/v1/auth/kubernetes/login \
     -d "{\"role\": \"admin\", \"jwt\": \"$K8S_TOKEN\"}" \
     -k | jq -r '.auth.client_token')
   
   # Use the token for OpenBao API calls
   curl -H "X-Vault-Token: $BAO_TOKEN" \
     https://bao-full.adfinis.localhost/v1/sys/health \
     -k
   ```

**Note:** The `-k` flag is used to skip TLS certificate verification, which is
necessary when using self-signed certificates (as configured in the test cluster).
In production, use proper certificates and remove this flag.

This approach allows you to obtain admin tokens without ever persisting
passwords or tokens in the OpenBaoCluster spec, as the root token is automatically
revoked when self-initialization is enabled.
