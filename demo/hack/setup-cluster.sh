#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

CONTAINER_BUILDER=${CONTAINER_BUILDER:-docker}
CERT_MANAGER_VERSION=${CERT_MANAGER_VERSION:-v1.16.3}
CLUSTER_NAME="mpc-v2-poc"
KUEUE_VERSION="v0.13.4"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$(dirname "$SCRIPT_DIR")")"
DEMO_DIR="$(realpath "${PROJECT_ROOT}/demo")"

log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required tools are installed
check_prerequisites() {
    log "Checking prerequisites..."
    
    if ! command -v kind &> /dev/null; then
        error "kind is not installed. Please install kind first."
        echo "Install with: go install sigs.k8s.io/kind@v0.20.0"
        exit 1
    fi
    
    if ! command -v kubectl &> /dev/null; then
        error "kubectl is not installed. Please install kubectl first."
        exit 1
    fi
        
    log "All prerequisites are installed"
}

# Generate code and manifests for each component
generate_manifests_all() {
  make -C may generate manifests
  make -C drivers/incluster generate manifests
}

# Checks all components vets
vet_all() {
  make -C may vet
  make -C drivers/incluster vet
}

# Create kind cluster
create_cluster() {
    log "Creating kind cluster '$CLUSTER_NAME'..."
    
    # Check if cluster already exists
    if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        warn "Cluster '$CLUSTER_NAME' already exists. Deleting..."
        kind delete cluster --name "$CLUSTER_NAME"
    fi
    
    # Create cluster with custom config
    kind create cluster --name "$CLUSTER_NAME"
    
    log "Cluster created successfully"
}

# Install Kueue
install_kueue() {
    log "Installing Kueue version $KUEUE_VERSION..."
    
    # Install Kueue CRDs and manager
    # kubectl apply --server-side -f "https://github.com/kubernetes-sigs/kueue/releases/download/${KUEUE_VERSION}/manifests.yaml"
    kubectl apply --server-side -k "${DEMO_DIR}/dependencies/kueue"
    
    # Wait for Kueue to be ready
    log "Waiting for Kueue to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/kueue-controller-manager -n kueue-system
    
    log "Kueue installed successfully"
}

# Install Cert Manager
install_cert_manager() {
  kubectl apply --server-side -f "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
  kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=300s
}

# Install Tekton-Kueue
install_tekton_kueue() {
    # Install Kueue CRDs and manager
    log "Installing Tekton-Kueue..."
    kustomize build "${DEMO_DIR}"/dependencies/tekton-kueue/config | kubectl apply -f - 
    
    # Wait for Kueue manager to be ready
    log "Waiting for Tekton Kueue to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/tekton-kueue-controller-manager -n tekton-kueue
    
    log "Tekton-Kueue installed successfully"
}

# Install Tekton Pipelines
install_tekton_pipelines() {
    log "Installing Tekton Pipelines..."
    kubectl apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml 

    log "Waiting for Tekton Pipelines to be ready..."
    kubectl wait --for=condition=Available deployment --all --timeout 300s -n tekton-pipelines
    kubectl wait --for=condition=Available deployment --all --timeout 300s -n tekton-pipelines

    log "Tekton Pipelines installed successfully..."
}

# Install MAY
install_may() {
    log "Installing MAY..."
    
    # Install May Scheduler CRDs and manager
    IMG="may:latest"
    IMG="${IMG}" make -C may docker-build
    kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"
    IMG="${IMG}" make -C may install deploy

    # Wait for May to be ready
    log "Waiting for May to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/may-controller-manager -n may-system
    
    log "May installed successfully"
}

# Install InCluster Driver
install_incluster_driver() {
    log "Installing InCluster Driver..."

    # Install InCluster Driver and manager
    IMG=may-incluster-driver:latest
    IMG="${IMG}" make -C drivers/incluster docker-build
    kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"
    IMG="${IMG}" make -C drivers/incluster install deploy

    # Wait for InCluster Driver to be ready
    log "Waiting for InCluster Driver to be ready..."
    kubectl wait --for=condition=available --timeout=300s deployment/incluster-incluster-driver -n may-system
    
    log "InCluster Driver installed successfully"
}

install_mpc_otp_server() {
  log "Applying Multi-Platform OTP Server"
  kustomize build "${DEMO_DIR}/dependencies/multi-platform-controller/config/otp" | \
    kubectl apply -f -

  log "Waiting for Multi-Platform OTP Server to be ready..."
  kubectl wait --for=condition=Available deployment --all --timeout 300s -n may-system
  log "Multi-Platform OTP Server Installed"
}

# Apply cohort
apply_cohorts() {
    log "Applying cohort configurations..."
    
    if [ -d "$DEMO_DIR/config/cohorts" ]; then
        kubectl apply -k "$DEMO_DIR/config/cohorts"
        log "Cohorts configurations applied"
    else
        warn "No cohort configurations found at $DEMO_DIR/config/cohorts"
    fi
}

# Apply Hosts
apply_hosts() {
    log "Applying hosts configurations..."
    kubectl apply -k "$DEMO_DIR/config/static/hosts/arm64"
    kubectl apply -k "$DEMO_DIR/config/dynamic/hostautoscaler/amd64"
    log "Policies configurations applied"
}

build_and_load_kubectl_openssh_image() {
  log "Building kubectl-openssh image"
  ${CONTAINER_BUILDER} build \
    -t kubectl-openssh:latest \
    -f "${DEMO_DIR}/dependencies/kubectl-openssh-image/Dockerfile" \
    "${DEMO_DIR}/dependencies/kubectl-openssh-image/"

  log "Loading kubectl-openssh image into Kind cluster"
  kind load docker-image kubectl-openssh:latest --name "${CLUSTER_NAME}"
}


# Main execution
main() {
    log "Starting MAY PoC..."
    
    check_prerequisites
    generate_manifests_all
    vet_all
    create_cluster
    install_cert_manager
    install_mpc_otp_server
    build_and_load_kubectl_openssh_image
    install_tekton_pipelines
    install_kueue
    install_tekton_kueue
    install_may
    install_incluster_driver
    apply_cohorts
    # apply_hosts
    
    log "Setup completed! Cluster info:"
    kubectl cluster-info --context "kind-$CLUSTER_NAME"
    
    log "To use this cluster, run: kubectl config use-context kind-$CLUSTER_NAME"
    log "To delete this cluster, run: kind delete cluster --name $CLUSTER_NAME"
}

# Cleanup function
cleanup() {
    if [ "$1" = "--cleanup" ] || [ "$1" = "-c" ]; then
        log "Cleaning up cluster '$CLUSTER_NAME'..."
        kind delete cluster --name "$CLUSTER_NAME" || true
        log "Cleanup completed"
        exit 0
    fi
}

# Handle cleanup argument
cleanup "$1"

# Run main function
main
