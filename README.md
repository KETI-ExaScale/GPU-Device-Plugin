# KETI-ExaScale GPU-Device-Plugin
## Introduction of KETI-ExaScale GPU-Device-Plugin
GPU-Device-Plugin for KETI-ExaScale Platform

Developed by KETI
## Contents
[1. Requirment](#requirement)

[2. How to Install](#how-to-install)

[3. Install Check](#install-check)

[4. Governance](#governance)
## Requirement
> Kubernetes <= 1.24

> Nvidia-Driver >= 495.44

> Nvidia-Docker >= 2

> KETI-ExaScale GPU-Scheduler

> KETI-ExaScale GPU-Metric-Collector

> InfluxDB
## How to Install
    $ kubectl apply -f manifests/device-pluginaccount.yaml
## Install Check
Create Check

    $ kubectl get pods -A
    NAMESPACE     NAME                                  READY   STATUS      RESTARTS      AGE
    gpu           keti-gpu-device-plugin-t5j9k          2/2     Running     0             36s
Log Check

    $ kubectl logs [keti-gpu-device-plugin] -n gpu
    2021/12/28 11:03:49 Start KETI GPU device plugin
    2021/12/28 11:03:49 Loading NVML
    2021/12/28 11:03:51 Fetching devices.
    Set GPU ComputeMode by Index : 0
    Set GPU ComputeMode by Index : 1
