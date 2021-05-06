#!/bin/bash

# 删除 mpa admission 控制器的证书
set -e

echo "Deleting MPA Admission Controller certs."
kubectl delete secret --namespace=kube-system mpa-tls-certs
