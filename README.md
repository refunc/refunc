# Refunc

Refunc is a painless serverless platform to run [AWS Lambda](https://docs.aws.amazon.com/lambda/latest/dg/welcome.html) on k8s

![refunc arch](https://user-images.githubusercontent.com/354668/50409374-188daf80-082d-11e9-9a9b-77407cd196ed.png)

## Features

* Easy of use - ebrace the big ecosystem thru AWS Lambda compatible API and runtimes
* Portable - run everywhere that has kubernetes
* Scale to zero - auto scale from zero to many and vice verse
* Extensible - runtime compatible layer(lamdba and other clouds' fucntion), transport layer(nats based for now)

## Quick Start

Before start, you need a k8s runs on somewhere, [minikube](https://github.com/kubernetes/minikube) is pretty enough, [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) with kubernetes enabled is recommended if your'd like to try on macOS.

### Install Refunc

Install Refunc play(which is a mini setup of refunc) using the following commands:

```shell
# This will create namespace `refunc-play` and deploy components in it
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc \
refunc play gen -n refunc-play | kubectl apply -f -
# create runtime python3.7
kubectl create -n refunc-play -f \
https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.2/xenv.yaml
```

Here we use python3.7 as an example, currently refunc support all [aws provided runtimes](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-custom.html) as well as some converted, check [refunc/lambda-runtimes](https://github.com/refunc/lambda-runtimes) to learn more.

### The aws way

Refunc using a AWS api compatible [gateway](https://github.com/refunc/aws-api-gw) to provide Lambda and S3 services,
this make it possible to use asw cli to managment functions directly.

Brefore start, we need forward gateway to local

```shell
kubectl port-forward deployment/aws-api-gw 9000:80 -n refunc-play
```

Download prebuild [Function](https://github.com/refunc/lambda-python3.7-example) for convenience

```shell
cd /tmp
wget https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.1/lambda.zip
```

#### Create python3.7 funtion

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda create-function --function-name localtest \
--handler lambda_function.lambda_handler \
--zip-file fileb:///tmp/lambda.zip \
--runtime python3.7 \
--role arn:aws:iam::XXXXXXXXXXXXX:role/your_lambda_execution_role
```

#### Invoke

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda invoke --function-name localtest /tmp/output.json && cat /tmp/output.json
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

## User interface

Internally we use [Rancher](https://rancher.com) to build our PaaS and other internal services, currently there is a simple management [UI](https://github.com/refunc/refunc-ui) forked from [rancher/ui](https://github.com/rancher/ui) which is backed by our [Rancher API](https://github.com/rancher/api-spec) compatible [server](https://github.com/refunc/refunc-rancher)

![functions.png](https://user-images.githubusercontent.com/354668/44694551-b13f3900-aaa0-11e8-8a9a-a19d562ec8d1.png "Functions page")

## License

Copyright (c) 2018 [refunc.io](http://refunc.io)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
