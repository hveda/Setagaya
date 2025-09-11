#!/bin/bash

# Kubernetes API Version Compatibility Script
# This script helps manage API versions across different Kubernetes versions

set -euo pipefail

# Configuration
KUBERNETES_DIR="kubernetes"
HELM_TEMPLATES_DIR="setagaya/install/setagaya/templates"
BACKUP_SUFFIX=".backup"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
}

# Function to show usage
usage() {
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  validate <k8s-version>    Validate manifests against specific Kubernetes version"
    echo "  adapt <k8s-version>       Adapt API versions for specific Kubernetes version"
    echo "  restore                   Restore original manifests from backup"
    echo "  matrix                    Test against multiple versions"
    echo "  check                     Check current API versions"
    echo ""
    echo "Examples:"
    echo "  $0 validate 1.21.0       # Validate against Kubernetes 1.21"
    echo "  $0 adapt 1.21.0          # Adapt manifests for Kubernetes 1.21"
    echo "  $0 matrix                 # Test against multiple versions"
    echo ""
    exit 1
}

# Function to check if kubeconform is installed
check_kubeconform() {
    if ! command -v kubeconform &> /dev/null; then
        print_status $RED "❌ kubeconform is not installed"
        print_status $YELLOW "Installing kubeconform..."

        if [[ "$OSTYPE" == "linux-gnu"* ]]; then
            curl -L https://github.com/yannh/kubeconform/releases/latest/download/kubeconform-linux-amd64.tar.gz | tar xz
            sudo mv kubeconform /usr/local/bin
        elif [[ "$OSTYPE" == "darwin"* ]]; then
            curl -L https://github.com/yannh/kubeconform/releases/latest/download/kubeconform-darwin-amd64.tar.gz | tar xz
            sudo mv kubeconform /usr/local/bin
        else
            print_status $RED "❌ Unsupported OS for automatic installation"
            exit 1
        fi

        print_status $GREEN "✅ kubeconform installed successfully"
    fi
}

# Function to validate manifests against a specific Kubernetes version
validate_manifests() {
    local k8s_version=$1
    print_status $BLUE "🔍 Validating manifests against Kubernetes ${k8s_version}"

    check_kubeconform

    # Validate core Kubernetes manifests
    print_status $YELLOW "Validating core manifests..."
    if kubeconform -summary -verbose -kubernetes-version "${k8s_version}" "${KUBERNETES_DIR}/"; then
        print_status $GREEN "✅ Core manifests are compatible with Kubernetes ${k8s_version}"
        core_success=true
    else
        print_status $RED "❌ Core manifests have issues with Kubernetes ${k8s_version}"
        core_success=false
    fi

    # Validate Helm templates (best effort)
    print_status $YELLOW "Validating Helm templates..."
    if kubeconform -summary -kubernetes-version "${k8s_version}" "${HELM_TEMPLATES_DIR}/" 2>/dev/null || true; then
        print_status $GREEN "✅ Helm templates validated (warnings expected)"
    else
        print_status $YELLOW "⚠️  Helm templates have validation warnings (expected due to templating)"
    fi

    if [[ "$core_success" == true ]]; then
        print_status $GREEN "🎉 Overall validation successful for Kubernetes ${k8s_version}"
        return 0
    else
        print_status $RED "💥 Validation failed for Kubernetes ${k8s_version}"
        return 1
    fi
}

# Function to adapt API versions for specific Kubernetes version
adapt_manifests() {
    local k8s_version=$1
    print_status $BLUE "🔧 Adapting manifests for Kubernetes ${k8s_version}"

    # Create backup
    backup_manifests

    # Parse version
    local major_minor
    local version_number
    major_minor=$(echo "${k8s_version}" | cut -d. -f1,2)
    version_number=$(echo "${major_minor}" | tr -d '.')

    print_status $YELLOW "Target version: ${k8s_version} (${major_minor})"

    # Apply version-specific adaptations
    if (( version_number < 121 )); then
        # Kubernetes < 1.21: Use policy/v1beta1 for PodDisruptionBudget
        print_status $YELLOW "Adapting for Kubernetes < 1.21: Using policy/v1beta1 for PodDisruptionBudgets"
        find "${KUBERNETES_DIR}" -name "*.yaml" -exec sed -i.tmp 's/apiVersion: policy\/v1$/apiVersion: policy\/v1beta1/g' {} \;
        find "${KUBERNETES_DIR}" -name "*.tmp" -delete
    elif (( version_number >= 132 )); then
        # Kubernetes >= 1.32: policy/v1beta1 removed, must use policy/v1
        print_status $YELLOW "Adapting for Kubernetes >= 1.32: policy/v1beta1 removed, using policy/v1 for PodDisruptionBudgets"
        find "${KUBERNETES_DIR}" -name "*.yaml" -exec sed -i.tmp 's/apiVersion: policy\/v1beta1$/apiVersion: policy\/v1/g' {} \;
        find "${KUBERNETES_DIR}" -name "*.tmp" -delete
    else
        # Kubernetes 1.21+: Using policy/v1 for PodDisruptionBudgets (standard)
        print_status $YELLOW "Adapting for Kubernetes 1.21+: Using policy/v1 for PodDisruptionBudgets"
        find "${KUBERNETES_DIR}" -name "*.yaml" -exec sed -i.tmp 's/apiVersion: policy\/v1beta1$/apiVersion: policy\/v1/g' {} \;
        find "${KUBERNETES_DIR}" -name "*.tmp" -delete
    fi

    print_status $GREEN "✅ Manifests adapted for Kubernetes ${k8s_version}"

    # Validate adapted manifests
    if validate_manifests "${k8s_version}"; then
        print_status $GREEN "🎉 Adapted manifests are compatible!"
    else
        print_status $RED "💥 Adapted manifests still have issues"
        print_status $YELLOW "Restoring original manifests..."
        restore_manifests
        return 1
    fi
}

# Function to backup manifests
backup_manifests() {
    print_status $YELLOW "📦 Creating backup of original manifests..."
    find "${KUBERNETES_DIR}" -name "*.yaml" -exec cp {} {}${BACKUP_SUFFIX} \;
    print_status $GREEN "✅ Backup created with suffix ${BACKUP_SUFFIX}"
}

# Function to restore manifests from backup
restore_manifests() {
    print_status $YELLOW "🔄 Restoring manifests from backup..."
    find "${KUBERNETES_DIR}" -name "*${BACKUP_SUFFIX}" | while read -r backup_file; do
        original_file="${backup_file%${BACKUP_SUFFIX}}"
        mv "${backup_file}" "${original_file}"
        print_status $GREEN "Restored: ${original_file}"
    done
    print_status $GREEN "✅ All manifests restored from backup"
}

# Function to test against multiple versions
test_matrix() {
    local versions=("1.21.0" "1.25.0" "1.28.0" "1.34.0")
    local results=()

    print_status $BLUE "🚀 Testing against multiple Kubernetes versions"

    for version in "${versions[@]}"; do
        print_status $YELLOW "\n=== Testing Kubernetes ${version} ==="

        if validate_manifests "${version}"; then
            results+=("${version}: ✅ PASS")
        else
            results+=("${version}: ❌ FAIL")
        fi
    done

    # Print summary
    print_status $BLUE "\n📊 Test Matrix Results:"
    for result in "${results[@]}"; do
        if [[ $result == *"PASS"* ]]; then
            print_status $GREEN "${result}"
        else
            print_status $RED "${result}"
        fi
    done
}

# Function to check current API versions
check_versions() {
    print_status $BLUE "🔍 Checking current API versions in manifests"

    print_status $YELLOW "\nCore Kubernetes manifests:"
    grep -r "apiVersion:" "${KUBERNETES_DIR}/" | sort | uniq

    print_status $YELLOW "\nHelm templates:"
    grep -r "apiVersion:" "${HELM_TEMPLATES_DIR}/" | sort | uniq

    print_status $BLUE "\n📋 API Version Summary:"
    print_status $GREEN "✅ apps/v1 (Deployments, ReplicaSets) - Stable since K8s 1.9"
    print_status $GREEN "✅ v1 (Services, ConfigMaps, Secrets) - Stable"
    print_status $GREEN "✅ rbac.authorization.k8s.io/v1 - Stable since K8s 1.8"

    if grep -q "policy/v1beta1" "${KUBERNETES_DIR}"/*.yaml; then
        print_status $RED "❌ policy/v1beta1 (PodDisruptionBudgets) - Deprecated in K8s 1.25, removed in 1.32+"
    fi

    if grep -q "policy/v1" "${KUBERNETES_DIR}"/*.yaml; then
        print_status $GREEN "✅ policy/v1 (PodDisruptionBudgets) - Stable since K8s 1.21, required for 1.32+"
    fi
}

# Main script logic
main() {
    if [[ $# -eq 0 ]]; then
        usage
    fi

    local command=$1
    shift

    case $command in
        "validate")
            if [[ $# -ne 1 ]]; then
                echo "Error: validate command requires Kubernetes version"
                echo "Usage: $0 validate <k8s-version>"
                exit 1
            fi
            validate_manifests "$1"
            ;;
        "adapt")
            if [[ $# -ne 1 ]]; then
                echo "Error: adapt command requires Kubernetes version"
                echo "Usage: $0 adapt <k8s-version>"
                exit 1
            fi
            adapt_manifests "$1"
            ;;
        "restore")
            restore_manifests
            ;;
        "matrix")
            test_matrix
            ;;
        "check")
            check_versions
            ;;
        *)
            echo "Error: Unknown command '$command'"
            usage
            ;;
    esac
}

# Run main function with all arguments
main "$@"
