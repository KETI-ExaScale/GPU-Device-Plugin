#!/usr/bin/env bash
password="ketilinux"
ip="10.0.5.24"

#$1 " " or deployment

if [ "$1" == "deployment" ] || [ "$1" == "d" ]; then   
    dest_path="/root/workspace/jhk/gpu-device-plugin"
    echo scp deployments root@$ip:$dest_path copying...
    sshpass -p $password scp -r deployments root@$ip:$dest_path
else
    dest_path="/root/workspace/jhk/gpu-device-plugin/deployments"
    echo scp /deployments/keti-gpu-device-plugin.yaml root@$ip:$dest_path copying...
    sshpass -p $password scp /deployments/keti-gpu-device-plugin.yaml root@$ip:$dest_path
fi