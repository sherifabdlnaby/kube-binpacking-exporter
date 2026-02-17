#!/bin/bash
set -e

echo "=== Kubernetes Connectivity Test ==="
echo ""

echo "1. Checking kubeconfig..."
if [ -f ~/.kube/config ]; then
    echo "   ✓ ~/.kube/config exists"
else
    echo "   ✗ ~/.kube/config not found"
    exit 1
fi

echo ""
echo "2. Checking current context..."
CONTEXT=$(kubectl config current-context 2>&1)
if [ $? -eq 0 ]; then
    echo "   ✓ Current context: $CONTEXT"
else
    echo "   ✗ No current context set"
    echo "   Available contexts:"
    kubectl config get-contexts
    exit 1
fi

echo ""
echo "3. Testing API server connectivity..."
if kubectl cluster-info &>/dev/null; then
    echo "   ✓ Can reach API server"
    kubectl cluster-info | head -2
else
    echo "   ✗ Cannot reach API server"
    kubectl cluster-info
    exit 1
fi

echo ""
echo "4. Testing node access..."
NODE_COUNT=$(kubectl get nodes --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "$NODE_COUNT" -gt 0 ]; then
    echo "   ✓ Can list nodes (found $NODE_COUNT nodes)"
    kubectl get nodes --no-headers | head -3
else
    echo "   ✗ Cannot list nodes or no nodes found"
    exit 1
fi

echo ""
echo "5. Testing pod access..."
POD_COUNT=$(kubectl get pods -A --no-headers 2>/dev/null | wc -l | tr -d ' ')
if [ "$POD_COUNT" -gt 0 ]; then
    echo "   ✓ Can list pods (found $POD_COUNT pods)"
else
    echo "   ⚠ Cannot list pods or no pods found"
fi

echo ""
echo "=== All checks passed! ==="
echo ""
echo "You can now run the exporter:"
echo "  ./kube-cluster-binpacking-exporter"
echo ""
echo "Or with debug logging:"
echo "  ./kube-cluster-binpacking-exporter --debug"
