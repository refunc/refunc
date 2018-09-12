# 快速开始

## 部署

### 在 Minikube 中部署

为了方便本地测试, Refunc 为 [Minikube](https://github.com/kubernetes/minikube) 和 [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) 预先生成了[所有组件的yaml](https://appstatics.oss-cn-shanghai.aliyuncs.com/refunc/start/play-local.yaml), 在终端里执行下面的命令:

```bash
curl -Ls https://bit.ly/2N7DdE1 | kubectl create -f -
```

如果一切顺利的话此时 Refunc 已经在本地的 Kubernetes 里跑起来了:

```bash
$ kubectl get deployments,services -n refunc
NAME                                   DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.extensions/controllers      1         1         1            1           30s
deployment.extensions/eventtrigger     1         1         1            1           30s
deployment.extensions/gateway          1         1         1            1           30s
deployment.extensions/s3               1         1         1            1           30s
deployment.extensions/timetrigger      1         1         1            1           30s
deployment.extensions/xpool-python36   2         2         2            2           26s

NAME                  TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)                       AGE
service/gateway       NodePort    10.101.164.71    <none>        80:30091/TCP,4222:30092/TCP   30s
service/nats          ClusterIP   None             <none>        4222/TCP,6222/TCP,8222/TCP    30s
service/nats-96       ClusterIP   10.96.99.202     <none>        4222/TCP                      30s
service/s3            ClusterIP   10.96.99.201     <none>        80/TCP                        30s
service/s3-nodeport   NodePort    10.103.244.165   <none>        80:30090/TCP                  30s
```

当然, 由于演示定律的存在上面的过程也有可能出错, 例如容易出现若干类似下面的问题

> Error from server (NotFound): error when creating "STDIN": the server could not find the requested resource (post funcdeves.k8s.refunc.io)

这是由于k8s在创建Refunc资源的时候[CRD](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)还没有创建好, 不过没关系, 我们再 `apply` 下应该就好了

```bash
curl -Ls https://bit.ly/2N7DdE1 | kubectl apply -f -
```

#### 一键删除

```bash
curl -Ls https://bit.ly/2N7DdE1 | kubectl delete -f -
```

::: tip
Refunc 可以使用 Helm Chart 在线上的集群中部署, 目前还有一些工作, Helm Chart 会随主工程开源后一起提供
:::

## 安装命令行

Refunc 提供一个基于Python的命令行工具[rfctl](https://github.com/refunc/py-rfctl)来管理云函数, 使用 `pip` 来安装它:

```bash
pip install rfctl
```

`rfctl` 在安装的时候会自动安装依赖[py-refunc](https://github.com/refunc/py-refunc), 它是Refunc 的Python云函数运行时库, 同时支持本地开发, 用法后面会详细介绍

## 配置客户端

`rfctl` 安装完成后还需要简单的配置才能使用 :smile:

预生成的配置文件默认生成了简单的用户信息放到 `refunc/creds-admin` 的 `ConfigMap` 中:

```bash
$ kubectl get configmaps creds-admin -n refunc -o yaml --export
apiVersion: v1
kind: ConfigMap
metadata:
    refunc.io/is-credential-config: "true"
  creationTimestamp: null
  labels:
    app: refunc
    chart: refunc-0.1.3
    heritage: Tiller
    release: refunc
  name: creds-admin
  selfLink: /api/v1/namespaces/refunc/configmaps/creds-admin
data:
  id: admin
  accessKey: AKIAIOSFODNN7EXAMPLE
  secretKey: wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY
  permissions: |
    {
        "publish": ["refunc.>", "_refunc.>"],
        "subscribe": ["_INBOX.*.*","_INBOX.*", "refunc.>", "_refunc.>"]
    }
```

::: warning
特别注意: 上面出现的 `accessKey` 和 `secretKey` 仅为了本地测试使用, 不要在其他环境中误用
:::

我们需要将上面的信息写到本地的配置文件中: `${HOME}/.refunc/config.yaml`:

::: warning
特别注意: 执行下面命令会覆盖原来的配置文件, 如果你之前已经有企业版的配置文件, 现在只是要测试下的话, 记得备份原来的配置
:::

```bash
# 创建目录
mkdir -p ${HOME}/.refunc || true

# 配置写到 config.yaml
cat <<EOF >${HOME}/.refunc/config.yaml
apiURL: nats://127.0.0.1:30092
username: admin
credentials:
  accessKey: AKIAIOSFODNN7EXAMPLE
  secretKey: wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY
EOF
```

### 测试下

走到这大概率我们的系统就部署好了, 终于可以测试下了

::: tip
如果基于Minikube部署的, nodePort默认无法通过本地回环地址访问, 你需要找到当前Minikube虚拟机的IP地址
```bash
export REFUNC_GATEWAY_URL=nats://${minikube ip}:30092
```
:::

确认现在系统 Python 版本至少是 3.6

```bash
python -c 'import refunc, json; print(json.dumps(refunc.invoke("buildins/cluster-info")))'
```

如果不出意外你将看到类似下面的输出:

> {"bundleVersion": "6c0f75ff", "agentVersion": "1f6db1fa"}

## 创建第一个函数

使用 rfctl 创建一个云函数:

```bash
$ rfctl new functions/helloworld --runtime python --init-git=false
Creating "functions/helloworld" in Helloworld
```

此时会在当前路径下创建一个名为 `Helloworld` 的文件夹

```bash
$ ls Helloworld
README.md   main.py     refunc.yaml
```

rfctl 会自动生成一个模板工程, `main.py` 的内容为

```python
# -*- coding: utf-8 -*-

import refunc
from refunc import Context, Message


def on_request(ctx: Context, payload: dict):
    pass


if __name__ == '__refunc_dev__':
    # run using:
    # refunc-dev main.py

    import refunc
    from refunc.util import enable_logging

    # setup mock
    enable_logging()
    # set mock endpoint
    refunc.current_env().context.set_mock_endpoint('functions/helloworld')

    # add extra mock points, for example:
    # comment out the following lines, a function at "foo/bar" can be mocked
    #
    # current_env().context.add_mock_func('foo/bar', lambda c, kv: kv)

    def simple_test(**kwargs: dict):
        return refunc.invoke("functions/helloworld", **kwargs)

    try:
        from IPython import embed
        __name__ = '__main__'  # fix warning
        embed()
    except:
        print("cannot drop into ipython, exec simple_test")
        print(simple_test())
```

我们修改下 `on_reqeust` 的实现为:

```python
def on_request(ctx: Context, payload: dict):
    return {"message": f"Hi {payload.get('name')}!"}
```

在 `Helloworld` 文件夹内执行命令提交函数

```bash
$ rfctl apply .
Packing "functions/helloworld"
Applying "functions/helloworld"
[  >>] 0912T11:32:45.995Z refunc/builder] [fn I] extracting source files
[  >>] 0912T11:32:46.006Z refunc/builder] [fn I] loading .env
[  >>] 0912T11:32:46.007Z refunc/builder] [fn I] uploading package
[  >>] 0912T11:32:46.134Z refunc/builder] [fn I] start building
[  >>] 0912T11:32:46.526Z refunc/builder] [fn I] python:build| compiling to pyc
[  >>] 0912T11:32:46.527Z refunc/builder] [fn I] python:build| Listing '.'...
[  >>] 0912T11:32:46.527Z refunc/builder] [fn I] python:build| Compiling './main.py'...
[  >>] 0912T11:32:46.527Z refunc/builder] [fn I] building done
[  >>] 0912T11:32:46.528Z refunc/builder] [fn I] packing
[  >>] 0912T11:32:46.546Z refunc/builder] [fn I] done: functions/helloworld/blobs/ab2c401b286a5dadf59e18a4282a2a33.tar.gz
[  >>] 0912T11:32:46.548Z refunc/builder] [fn I] applying func 'functions/helloworld'
Done "functions/helloworld"
```

好了, 到目前为止我们已经实现了一个云函数, 在代码里调用试一试:

```bash
python -c 'import refunc; print(refunc.invoke("functions/helloworld", name="ZhaoBin")["message"])'
```

也可以在 python 的 console 或者 iPython 里交互的调用

```shell
$ ipython
Python 3.6.6 |Anaconda, Inc.| (default, Jun 28 2018, 11:07:29)
Type 'copyright', 'credits' or 'license' for more information
IPython 6.4.0 -- An enhanced Interactive Python. Type '?' for help.

In [1]: import refunc

In [2]: refunc.invoke("functions/helloworld", name="ZhaoBin")
Out[2]: {'message': 'hi ZhaoBin!'}
```