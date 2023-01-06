#!/usr/bin/env bash

#$1 create/c or delete/d

if [ "$1" == "delete" ] || [ "$1" == "d" ]; then   
    echo kubectl delete -f deployments/keti-gpu-device-plugin.yaml
    kubectl delete -f deployments/keti-gpu-device-plugin.yaml
else
    echo kubectl create -f deployments/keti-gpu-device-plugin.yaml
    kubectl create -f deployments/keti-gpu-device-plugin.yaml
fi