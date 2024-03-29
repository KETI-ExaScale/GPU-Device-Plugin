#!/usr/bin/env bash

#$1 create/c or delete/d

if [ "$1" == "delete" ] || [ "$1" == "d" ]; then   
    echo kubectl delete -f deployments/gpu-device-plugin-service.yaml
    kubectl delete -f deployments/gpu-device-plugin-service.yaml
else
    echo kubectl apply -f deployments/gpu-device-plugin-service.yaml
    kubectl apply -f deployments/gpu-device-plugin-service.yaml
fi