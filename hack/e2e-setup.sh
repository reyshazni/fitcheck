#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-fitcheck-e2e}"
IMAGE_NAME="${IMAGE_NAME:-fitcheck:e2e}"
NAMESPACE="${NAMESPACE:-kube-system}"
CHART_DIR="${CHART_DIR:-charts/fitcheck}"

echo "Building fitcheck image..."
docker build -t "${IMAGE_NAME}" .

echo "Loading image into kind cluster..."
kind load docker-image "${IMAGE_NAME}" --name "${CLUSTER_NAME}"

echo "Installing fitcheck via Helm..."
helm upgrade --install fitcheck "${CHART_DIR}" \
  --namespace "${NAMESPACE}" \
  --set image.repository=fitcheck \
  --set image.tag=e2e \
  --set image.pullPolicy=Never \
  --wait \
  --timeout 120s

echo "Waiting for deployment to be ready..."
kubectl rollout status deployment/fitcheck -n "${NAMESPACE}" --timeout=120s

echo "E2E setup complete."
