# Refunc

Refunc is a [Kubernetes](https://kubernetes.io) native serverless platform.

![Refunc Architecture](https://user-images.githubusercontent.com/354668/50409374-188daf80-082d-11e9-9a9b-77407cd196ed.png)

## Features

* Easy of use - Embrace the serverless ecosystem with an AWS Lambda compatible API and runtimes
* Portable - Run everywhere that has Kubernetes
* Scale from zero - Autoscale from zero-to-many and vice versa
* Extensible - Runtime compatibility layer (lambda and other clouds' function), transport layer ([NATS](https://nats.io) based for now)

## Quick Start

Before starting, you need a Kubernetes cluster. You can use [minikube](https://github.com/kubernetes/minikube) to run a minimal Kubernetes cluster locally.

If you'd like to run on macOS, [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) with Kubernetes enabled is recommended.

### Install Refunc

Install `refunc-play`, a minimal setup of Refunc, using the following commands:

```shell
# This will create namespace `refunc-play` and deploy components in it
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc \
refunc play gen -n refunc-play | kubectl apply -f -
# create runtime python3.7
kubectl create -n refunc-play -f \
https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.2/xenv.yaml
```

Here we use the python3.7 runtime as an example. Currently Refunc supports all [AWS provided runtimes](https://docs.aws.amazon.com/lambda/latest/dg/runtimes-custom.html) as well as some converted runtimes, check [refunc/lambda-runtimes](https://github.com/refunc/lambda-runtimes) to learn more.

### The AWS Way

Refunc uses an AWS API compatible [gateway](https://github.com/refunc/aws-api-gw) to provide Lambda and S3 services,
which makes it possible to use the AWS CLI to manage functions locally.

Before starting, we need to forward the gateway to your localhost:

```shell
kubectl port-forward deployment/aws-api-gw 9000:80 -n refunc-play
```

Download the pre-built [function](https://github.com/refunc/lambda-python3.7-example) for convenience:

```shell
cd /tmp
wget https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.1/lambda.zip
```

#### Create Function

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda create-function --function-name localtest \
--handler lambda_function.lambda_handler \
--zip-file fileb:///tmp/lambda.zip \
--runtime python3.7 \
--role arn:aws:iam::XXXXXXXXXXXXX:role/your_lambda_execution_role
```

#### Invoke Function

```shell
aws --endpoint-url=http://127.0.0.1:9000 \
lambda invoke --function-name localtest /tmp/output.json && cat /tmp/output.json
```

### The Refunc Way

Let's create a Lambda function using the python3.7 runtime with an HTTP endpoint:

```shell
kubectl create -n refunc-play -f https://github.com/refunc/lambda-python3.7-example/releases/download/v0.0.2/inone.yaml
```

Forward the Refunc HTTP gateway to your localhost:

```shell
kubectl port-forward deployment/refunc-play 7788:7788 -n refunc-play
```

Now, it's OK to send a request to your function:

```shell
curl -v  http://127.0.0.1:7788/refunc-play/python37-function
```

## User Interface

Internally we use [Rancher](https://rancher.com) to build our PaaS and other internal services. Currently there is a simple management [UI](https://github.com/refunc/refunc-ui) forked from [rancher/ui](https://github.com/rancher/ui) which is backed by our [Rancher API](https://github.com/rancher/api-spec) compatible [server](https://github.com/refunc/refunc-rancher).

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
