# 使用 rfctl 管理

::: warning
特别注意: 下面文档描述功能在社区版可能并未完全支持
:::

Refunc 提供一个基于Python的命令行工具[rfctl](https://github.com/refunc/py-rfctl)来管理云函数

## 安装命令行

使用 `pip` 来安装它:

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
export REFUNC_GATEWAY_URL=nats://$(minikube ip):30092
```
:::

确认现在系统 Python 版本至少是 3.6

```bash
rfctl call builtins/cluster-info
```

如果不出意外你将看到类似下面的输出:

> {"refuncVersion": "6c0f75ff", "agentVersion": "1f6db1fa"}

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
rfctl call functions/helloworld --data '{"name": "ZhaoBin"}'
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
