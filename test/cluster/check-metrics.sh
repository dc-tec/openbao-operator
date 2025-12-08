#!/bin/bash
# Script to check if OpenBao Operator metrics are being collected

set -e

echo "=== Checking Metrics Service ==="
kubectl get svc -n openbao-operator-system | grep metrics || echo "No metrics service found"

echo ""
echo "=== Checking ServiceMonitor ==="
kubectl get servicemonitor -n openbao-operator-system -o yaml

echo ""
echo "=== Checking Prometheus Targets ==="
PROM_POD=$(kubectl get pods -n monitoring -l app.kubernetes.io/name=prometheus -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$PROM_POD" ]; then
  echo "Prometheus pod: $PROM_POD"
  echo "Checking targets..."
  kubectl exec -n monitoring $PROM_POD -- wget -qO- http://localhost:9090/api/v1/targets | grep -A 5 "openbao-operator" || echo "No openbao-operator targets found"
else
  echo "Prometheus pod not found"
fi

echo ""
echo "=== Checking Metrics Endpoint Directly ==="
METRICS_SVC=$(kubectl get svc -n openbao-operator-system -l app.kubernetes.io/name=openbao-operator -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$METRICS_SVC" ]; then
  echo "Metrics service: $METRICS_SVC"
  kubectl port-forward -n openbao-operator-system svc/$METRICS_SVC 8443:8443 &
  PF_PID=$!
  sleep 2
  echo "Testing metrics endpoint..."
  curl -k https://localhost:8443/metrics 2>/dev/null | grep -i "openbao" | head -5 || echo "No openbao metrics found"
  kill $PF_PID 2>/dev/null || true
else
  echo "Metrics service not found"
fi

echo ""
echo "=== Checking Prometheus RBAC ==="
kubectl get clusterrolebinding -o yaml | grep -A 10 "prometheus\|monitoring" | head -20 || echo "No Prometheus RBAC found"
