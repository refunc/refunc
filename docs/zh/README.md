---
home: true
actionText: 了解更多 →
actionLink: /zh/guide/
footer: Apache License v2.0 Licensed | Copyright © 2017-2019 Bin Zhao
---

### 快速开始

```shell
# install refunc play
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc refunc play gen -n refunc-play | kubectl apply -f -
```

::: tip
推荐在支持 Kubernetes 的 [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) 上尝鲜
:::

### 架构示意

![refunc arch](https://user-images.githubusercontent.com/354668/50409374-188daf80-082d-11e9-9a9b-77407cd196ed.png)