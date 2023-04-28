package play

import "text/template"

var k8sTpl = template.Must(template.New("k8s").Parse(`
---

apiVersion: v1
kind: Namespace
metadata:
  name: {{ .Namespace }}
spec:
  finalizers:
  - kubernetes

---

apiVersion: v1
kind: Secret
metadata:
  name: refunc
  namespace: {{ .Namespace }}
type: Opaque
data:
  minio-access-key: QUtJQUlPU0ZPRE5ON0VYQU1QTEU=
  minio-secret-key: d0phbHJYVXRuRkVNSUs3TURFTkdiUHhSZmlDWUVYQU1QTEVLRVk=
  access-token: dlNXcHdZa2xzZURGTlJFRjZXbE5LWkV4RFNucGtWMHA2

---

apiVersion: v1
kind: ConfigMap
metadata:
  name: refunc
  namespace: {{ .Namespace }}
data:
  nats.conf: |
    listen: 0.0.0.0:4222
    http: 0.0.0.0:8222

    authorization {
        token: vSWpwYklseDFNREF6WlNKZExDSnpkV0p6
    }

    debug:   true
    trace:   false
    logtime: true

    max_control_line: 1024

    ping_interval: 60

    # maximum payload 1MB
    max_payload: 1048576

    write_deadline: "2s"

  nginx.conf: |
    server {
        listen 80;
        access_log /dev/stdout;
        error_log /dev/stderr info;

        ignore_invalid_headers off;
        proxy_buffering off;

        location ^~ / {
          proxy_set_header Host $http_host;
          proxy_pass http://s3.{{.Namespace}};
        }
        location ^~ /2015-03-31/ {
          proxy_pass http://127.0.0.1:9000;
        }
    }

---

apiVersion: v1
kind: ServiceAccount
metadata:
  name: refunc
  namespace: {{ .Namespace }}

---

kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: refunc
  namespace: {{ .Namespace }}
subjects:
  - kind: ServiceAccount
    name: refunc
roleRef:
  kind: ClusterRole
  name: cluster-admin
  apiGroup: rbac.authorization.k8s.io

---

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: funcdeves.k8s.refunc.io
spec:
  group: k8s.refunc.io
  names:
    kind: Funcdef
    listKind: FuncdefList
    plural: funcdeves
    shortNames:
    - fnd
    singular: funcdef
  scope: Namespaced
  versions:
    - name: v1beta3
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true

---

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: funcinsts.k8s.refunc.io
spec:
  group: k8s.refunc.io
  names:
    kind: Funcinst
    listKind: FuncinstList
    plural: funcinsts
    shortNames:
    - fni
    singular: funcinst
  scope: Namespaced
  versions:
    - name: v1beta3
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true

---

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: triggers.k8s.refunc.io
spec:
  group: k8s.refunc.io
  names:
    kind: Trigger
    listKind: TriggerList
    plural: triggers
    shortNames:
    - tr
    singular: trigger
  scope: Namespaced
  versions:
    - name: v1beta3
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true

---

apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: xenvs.k8s.refunc.io
  labels:
spec:
  group: k8s.refunc.io
  names:
    kind: Xenv
    listKind: XenvList
    plural: xenvs
    shortNames:
    - xe
    singular: xenv
  scope: Namespaced
  versions:
    - name: v1beta3
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true

---

apiVersion: v1
kind: Service
metadata:
  name: nats
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: message
    refunc.io/name: nats
spec:
  selector:
    refunc.io/res: message
    refunc.io/name: nats
  ports:
  - name: client
    port: 4222

---

kind: Service
apiVersion: v1
metadata:
  name: s3
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: storage
    refunc.io/name: s3
spec:
  selector:
    refunc.io/res: storage
    refunc.io/name: s3
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: 9000

---

kind: Service
apiVersion: v1
metadata:
  name: refunc-http
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: play-in-one
spec:
  selector:
    refunc.io/res: play-in-one
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: 7788

---

kind: Service
apiVersion: v1
metadata:
  name: aws-api
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: gateway
    refunc.io/name: aws-api
spec:
  selector:
    refunc.io/res: gateway
    refunc.io/name: aws-api
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: 80

---

apiVersion: apps/v1
kind: Deployment
metadata:
  name: refunc-play
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: play-in-one
spec:
  replicas: 1
  selector:
    matchLabels:
      refunc.io/res: play-in-one
  template:
    metadata:
      labels:
        refunc.io/res: play-in-one
    spec:
      serviceAccount: refunc
      containers:
      - image: "refunc/refunc:{{ .ImageTag }}"
        imagePullPolicy: IfNotPresent
        name: controller
        env:
        - name: REFUNC_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: REFUNC_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        # the following are needed by runtime
        - name: NATS_ENDPOINT
          value: "nats.{{.Namespace}}:4222"
        - name: MINIO_ENDPOINT
          value: "http://s3.{{.Namespace}}"
        - name: MINIO_PUBLIC_ENDPOINT
          value: "http://s3.{{.Namespace}}"
        - name: MINIO_BUCKET
          value: {{ .Bucket }}
        - name: MINIO_SCOPE
          value: {{ .S3Prefix }}
        - name: MINIO_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-access-key
        - name: MINIO_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-secret-key
        - name: ACCESS_TOKEN
          valueFrom:
            secretKeyRef:
              name: refunc
              key: access-token
        command:
        - refunc
        - play
        - start
        - --v
        - "3"
        - -n
        - {{ .Namespace }}
        ports:
        - containerPort: 7788
          protocol: TCP

---

apiVersion: apps/v1
kind: Deployment
metadata:
  name: s3
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: storage
    refunc.io/name: s3
spec:
  replicas: 1
  selector:
    matchLabels:
      refunc.io/res: storage
      refunc.io/name: s3
  template:
    metadata:
      labels:
        refunc.io/res: storage
        refunc.io/name: s3
    spec:
      serviceAccount: refunc
      initContainers:
      - name: make-bucket
        image: busybox
        command:
        - mkdir
        - "-p"
        - "/export/refunc"
        volumeMounts:
        - name: export
          mountPath: /export
      containers:
      - image: minio/minio:RELEASE.2018-12-27T18-33-08Z
        imagePullPolicy: IfNotPresent
        name: minio
        env:
        - name: MINIO_UPDATE
          value: "off"
        - name: MINIO_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-access-key
        - name: MINIO_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-secret-key
        args:
          - server
          - /export
        volumeMounts:
        - name: export
          mountPath: /export
        ports:
        - containerPort: 9000
          protocol: TCP
      volumes:
      - name: export
        emptyDir: {}

---

apiVersion: apps/v1
kind: Deployment
metadata:
  name: nats-cluster
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: message
    refunc.io/name: nats
spec:
  replicas: 1
  selector:
    matchLabels:
      refunc.io/res: message
      refunc.io/name: nats
  template:
    metadata:
      labels:
        refunc.io/res: message
        refunc.io/name: nats
    spec:
      containers:
      - name: nats
        image: nats:2.6.2
        imagePullPolicy: IfNotPresent
        args:
        - "--config"
        - "/etc/nats/config/nats.conf"
        volumeMounts:
        - name: config-volume
          mountPath: /etc/nats/config
        ports:
        - containerPort: 4222
          name: client
        - containerPort: 6222
          name: cluster
        - containerPort: 8222
          name: monitor
        livenessProbe:
          httpGet:
            path: /
            port: 8222
          initialDelaySeconds: 10
          timeoutSeconds: 5
      volumes:
      - name: config-volume
        configMap:
          name: refunc
          items:
          - key: nats.conf
            path: nats.conf

---

apiVersion: apps/v1
kind: Deployment
metadata:
  name: aws-api-gw
  namespace: {{ .Namespace }}
  labels:
    refunc.io/res: gateway
    refunc.io/name: aws-api
spec:
  replicas: 1
  selector:
    matchLabels:
      refunc.io/res: gateway
      refunc.io/name: aws-api
  template:
    metadata:
      labels:
        refunc.io/res: gateway
        refunc.io/name: aws-api
    spec:
      serviceAccount: refunc
      containers:
      - name: api
        image: refunc/aws-api-gw
        imagePullPolicy: IfNotPresent
        command:
          - aws-api-gw
          - -n
          - {{ .Namespace }}
        env:
        - name: REFUNC_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: REFUNC_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        # the following are needed by runtime
        - name: NATS_ENDPOINT
          value: "nats.{{.Namespace}}:4222"
        - name: MINIO_ENDPOINT
          value: "http://s3.{{.Namespace}}"
        - name: MINIO_PUBLIC_ENDPOINT
          value: "http://s3.{{.Namespace}}"
        - name: MINIO_BUCKET
          value: {{ .Bucket }}
        - name: MINIO_SCOPE
          value: {{ .S3Prefix }}
        - name: MINIO_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-access-key
        - name: MINIO_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-secret-key
        - name: ACCESS_TOKEN
          valueFrom:
            secretKeyRef:
              name: refunc
              key: access-token
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-access-key
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: refunc
              key: minio-secret-key
        - name: S3_ENDPOINT
          value: "http://s3.{{.Namespace}}"
        - name: S3_REGION
          value: us-east-1
      - name: nginx
        image: nginx:1.15
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - name: config-volume
          mountPath: /etc/nginx/conf.d
        ports:
        - containerPort: 80
          name: http
      volumes:
      - name: config-volume
        configMap:
          name: refunc
          items:
          - key: nginx.conf
            path: default.conf
`))
