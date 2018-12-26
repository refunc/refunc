# Quick Start

Before start, you need a k8s runs on somewhere, [minikube](https://github.com/kubernetes/minikube) is pretty enough, [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) with kubernetes enabled is recommended if your'd like to try on macOS.

## Install Refunc

Install Refunc play(which is a mini setup of refunc) using the following commands:

```shell
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc \
refunc play gen -n refunc-play | kubectl apply -f -
```

This will create namespace `refunc-play` and deploy components in it.

## Install runtime python3.7

```shell
kubectl create -n refunc-play -f \
https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.2/xenv.yaml
```

::: tip
Refunc supports AWS provided runtime natively, and [other]((https://github.com/refunc/lambda-runtimes)) converted AWS language runtimes

Moreover, new runtimes can be easily by leverage refunc as a framework, [learn more](https://github.com/refunc/refunc/tree/master/pkg/runtime)
:::

## Play with function

### The AWS way

Forwarding refunc http gw to local in a seperate terminal:

```shell
kubectl port-forward deployment/aws-api-gw 9000:80 -n refunc-play
```

Download prebuild [Function](https://github.com/refunc/lambda-python3.7-example) for convenience

```shell
cd /tmp
wget https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.1/lambda.zip
```

Create python3.7 funtion

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda create-function --function-name localtest \
--handler lambda_function.lambda_handler \
--zip-file fileb:///tmp/lambda.zip \
--runtime python3.7 \
--role arn:aws:iam::XXXXXXXXXXXXX:role/your_lambda_execution_role
```

Invoke

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda invoke --function-name localtest /tmp/output.json && cat /tmp/output.json
```

### The Refunc way

Let's create a lambda function using runtime python3.7 with a http trigger:

```shell
kubectl create -n refunc-play -f https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.2/inone.yaml
```

Forwarding refunc http gw to local:

```shell
kubectl port-forward deployment/refunc-play 7788:7788 -n refunc-play
```

Now, it's OK to send a request to your function

```shell
curl -v  http://127.0.0.1:7788/refunc-play/python37-function
```

## User interface

Internally we use [Rancher](https://rancher.com) to build our PaaS and other internal services, and currently there is a simple management [UI](https://github.com/refunc/refunc-ui) forked from [rancher/ui](https://github.com/rancher/ui) which is backed with our [Rancher API](https://github.com/rancher/api-spec) compatible [server](https://github.com/refunc/refunc-rancher)

![functions.png](https://user-images.githubusercontent.com/354668/44694551-b13f3900-aaa0-11e8-8a9a-a19d562ec8d1.png "Functions page")