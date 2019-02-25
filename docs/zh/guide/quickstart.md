# 快速开始

## 安装 Refunc

### 在 Minikube 中部署

为了方便本地测试, Refunc 为 [Minikube](https://github.com/kubernetes/minikube) 和 [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) refunc 提供了 `play` 命令, 用来生成一个本地最小环境的k8s配置文件, 在终端里执行下面的命令:

```bash
docker run --rm -it refunc/refunc refunc play gen -n refunc-play | kubectl apply -f -
```

如果一切顺利的话此时 Refunc 已经在本地的 Kubernetes 里跑起来了:

```bash
$ kubectl get deployments,services -n refunc-play
NAME                                    DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.extensions/aws-api-gw        1         1         1            1           30s
deployment.extensions/nats-cluster      1         1         1            1           30s
deployment.extensions/refunc-play       1         1         1            1           30s
deployment.extensions/s3                1         1         1            1           30s

NAME                  TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/aws-api       ClusterIP   10.100.34.197    <none>        80/TCP     30s
service/nats          ClusterIP   10.110.36.186    <none>        4222/TCP   30s
service/refunc-http   ClusterIP   10.102.122.191   <none>        80/TCP     30s
service/s3            ClusterIP   10.110.184.0     <none>        80/TCP     30s
```

#### 一键删除

```bash
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc \
refunc play gen -n refunc-play | kubectl delete -f -
```

::: tip
Refunc 企业版提供基于 Helm Chart 在线上的集群中部署及多租户管理
:::

## 添加运行时

这里我们添加 AWS 的 python3.7

```shell
kubectl create -n refunc-play -f \
https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.2/xenv.yaml
```

::: tip
Refunc 支持所有基于 AWS provided 的运行时, 以及一部分AWS官方语言运行时, 具体[看这里](https://github.com/refunc/lambda-runtimes)
此外, 基于 Refunc 用户可以开发更多的运行时支持: [runtime](https://github.com/refunc/refunc/tree/master/pkg/runtime)
:::

## 创建云函数

### 使用 AWS Cli

Refunc 做了一个兼容 AWS Lambda 和 S3 的 [网关](https://github.com/refunc/aws-api-gw), 让用户可以使用 [aws-cli](https://aws.amazon.com/cli/) 无缝使用 Lambda 函数

在此之前需要现将网关的端口转发到本地

```shell
kubectl port-forward deployment/aws-api-gw 9000:80 -n refunc-play
```

为了方便演示, 下载预先打包好的函数 [Function](https://github.com/refunc/lambda-python3.7-example)

```shell
cd /tmp
wget https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.1/lambda.zip
```

::: tip
函数打包可以参考各种相关文档及使用相关工具
:::

创建 python3.7 的函数

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda create-function --function-name localtest \
--handler lambda_function.lambda_handler \
--zip-file fileb:///tmp/lambda.zip \
--runtime python3.7 \
--role arn:aws:iam::XXXXXXXXXXXXX:role/your_lambda_execution_role
```

调用

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda invoke --function-name localtest /tmp/output.json && cat /tmp/output.json
```

## 用户界面

在团队内部我们使用 [Rancher](https://rancher.com) 二次开发构建了整个中台, [这里](https://github.com/refunc/refunc-ui) 是 Refunc 集成到 [rancher/ui](https://github.com/rancher/ui) 的图形界面, 它用 [refunc-rancher](https://github.com/refunc/refunc-rancher) 为前端提供兼容  [Rancher API](https://github.com/rancher/api-spec) 的后端服务

![functions.png](https://user-images.githubusercontent.com/354668/44694551-b13f3900-aaa0-11e8-8a9a-a19d562ec8d1.png "Functions page")
