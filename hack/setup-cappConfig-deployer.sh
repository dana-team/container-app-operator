#!/bin/bash

make install  # Ensure CRD and related resources are installed
kubectl create ns container-app-operator-system || true  # Create namespace if it doesn't already exist
kubectl apply -f hack/manifests/cappconfig.yaml  # Apply CappConfig manifest