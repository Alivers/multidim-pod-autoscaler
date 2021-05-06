#!/bin/bash

# 在 k8s 集群中删除 admission 控制器的 webhook
set -e

echo "Unregistering MPA admission controller webhook"

kubectl delete -n kube-system mutatingwebhookconfiguration.v1beta1.admissionregistration.k8s.io mpa-webhook-config

