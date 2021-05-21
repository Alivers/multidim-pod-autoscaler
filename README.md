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

## 组件及其他说明

整个 MPA 由 Recommender、Updater、Admission 三个组件构成，三个组件独立部署，独立运行，之前的纽带为 某一时刻处于某一状态的MPA对象。

docker 镜像： https://hub.docker.com/u/aliverjon

```bash
# 每个组件修改完代码之后需要重新build、push镜像
# 如 Admission
make docker-push -C ./pkg/admission/
```