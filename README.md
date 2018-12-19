# Refunc

Refunc is a painless serverless platform that runs aws lambda runtimes(via lambci images) on k8s

## Quick Start

Before start, you need a k8s runs on somewhere, [minikube](https://github.com/kubernetes/minikube) is pretty enough, [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) with kubernetes enabled is recommended if your'd like to try on macOS.

Install Refunc play(which is a mini setup of refunc) using the following commands:

```shell
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc refunc play gen -n refunc-play | kubectl apply -f -
```

This will create namespace `refunc-play` and deploy refunc in it.

Let's create a lambda function using runtime python3.7 with a http trigger:

```shell
kubectl create -n refunc-play -f https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.1/inone.yaml
```

Forwarding refunc http gw to local:

```shell
kubectl port-forward deployment/refunc-play 7788:7788 -n refunc-play
```

Now, it's OK to send a request to your function

```shell
curl -v  http://127.0.0.1:7788/refunc-play/python37-function
```
