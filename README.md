# Kubernetes 组合式 Pod 伸缩器 —— MPA

MPA 即 Multidimensional Pod Autoscaler, 结合了水平和垂直两个维度的伸缩。

## Quick Start

### 在 k8s 集群中安装 MPA
```bash
cd multidim-pod-autoscaler
bash ./hack/mpa-up.sh
```

### 移除

```bash
bash ./hack/mpa-down.sh
```

### 测试

```bash
kubectl apply -f ./examples/cpu-bound/deploy-with-mpa.yaml
...
# 查看mpa对象
kubectl get mpa -o wide
```

## 目录结构说明

```bash
multidim-pod-autoscaler
├── deploy       # 集群部署的配置文件(yaml)
├── docs         # 开发参考文档
├── examples     # 测试样例
├── hack         # 部署、环境等相关的脚本
└── pkg          # 源码
```