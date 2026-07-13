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

echo "Labeling kind node with ACK nodepool label for provider detection..."
NODE_NAME=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
kubectl label node "${NODE_NAME}" alibabacloud.com/nodepool-id=e2e-pool --overwrite
kubectl label node "${NODE_NAME}" name=e2e-nodepool --overwrite

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
