---
home: true
actionText: Lean more →
actionLink: /en/guide/
footer: Apache License v2.0 Licensed | Copyright © 2017-2019 Bin Zhao
---

### Quick Start

```shell
# install refunc play
docker run --rm -it -e REFUNC_ENV=cluster refunc/refunc refunc play gen -n refunc-play | kubectl apply -f -
```

::: tip
For local play [Docker for Mac](https://docs.docker.com/docker-for-mac/kubernetes/) with kubernetes enabled is recommened
:::

### Architecture Overview

![refunc arch](https://user-images.githubusercontent.com/354668/50409374-188daf80-082d-11e9-9a9b-77407cd196ed.png)