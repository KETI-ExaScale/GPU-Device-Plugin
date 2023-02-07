#!/usr/bin/env bash
dest_path="/root/workspace/deployments/gpu-device-plugin"
password="ketilinux"
ip="10.0.5.120"

#$1 " " or deployment

if [ "$1" == "deployment" ] || [ "$1" == "d" ]; then   
    echo scp deployments root@$ip:$dest_path copying...
    sshpass -p $password scp -r deployments root@$ip:$dest_path
else
    echo scp /deployments/keti-gpu-device-plugin.yaml root@$ip:$dest_path/deployments copying...
    sshpass -p $password scp /deployments/keti-gpu-device-plugin.yaml root@$ip:$dest_path/deployments
fi