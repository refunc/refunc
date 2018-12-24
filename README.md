# Refunc

Refunc is a painless serverless platform that runs aws lambda runtimes(via lambci images) on k8s

![refunc arch](https://user-images.githubusercontent.com/354668/50403557-7a70f980-07da-11e9-905d-80c407a1cb18.png)

## Quick Start

Before start, you need a k8s runs on somewhere, [minikube](https://github.com/kubernetes/minikube) is pretty enough, [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) with kubernetes enabled is recommended if your'd like to try on macOS.

### Install Refunc

Install Refunc play(which is a mini setup of refunc) using the following commands:

```shell
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc refunc play gen -n refunc-play | kubectl apply -f -
# create runtime python3.7
kubectl create -n refunc-play -f https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.2/xenv.yaml
```

This will create namespace `refunc-play` and deploy components in it.

### The aws way

Forwarding refunc http gw to local in a seperate terminal:

```shell
kubectl port-forward deployment/aws-api-gw 9000:80 -n refunc-play
```

Download prebuild lambda.zip for convenience

```shell
wget https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.1/lambda.zip
```

Create python3.7 funtion

```shell
aws --endpoint-url=http://127.0.0.1:9000 lambda create-function --function-name localtest \
--handler lambda_function.lambda_handler \
--zip-file fileb:///tmp/lambda.zip \
--runtime python3.7 \
--role arn:aws:iam::XXXXXXXXXXXXX:role/your_lambda_execution_role
```

Invoke

```shell
aws --endpoint-url=http://127.0.0.1:9000 lambda invoke --function-name localtest /tmp/output.json && cat /tmp/output.json
```

### The refunc way

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
