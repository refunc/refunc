module github.com/refunc/refunc

go 1.12

require (
	github.com/allegro/bigcache v1.2.1
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/elazarl/goproxy v0.0.0-20190421051319-9d40249d3c2f // indirect
	github.com/elazarl/goproxy/ext v0.0.0-20190421051319-9d40249d3c2f // indirect
	github.com/fsnotify/fsnotify v1.5.1
	github.com/gabriel-vasile/mimetype v1.4.1
	github.com/garyburd/redigo v1.6.0
	github.com/go-ini/ini v1.42.0 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/google/uuid v1.1.2
	github.com/gopherjs/gopherjs v0.0.0-20190411002643-bd77b112433e // indirect
	github.com/gorilla/handlers v1.4.0
	github.com/gorilla/mux v1.7.1
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/mattn/go-shellwords v1.0.12
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/minio/minio-go v6.0.14+incompatible
	github.com/nats-io/nats-server/v2 v2.6.2 // indirect
	github.com/nats-io/nats.go v1.13.0
	github.com/nats-io/nuid v1.0.1
	github.com/nwaples/rardecode v1.0.0 // indirect
	github.com/pierrec/lz4 v2.0.5+incompatible // indirect
	github.com/refunc/go-observer v1.0.3
	github.com/robfig/cron v1.2.0
	github.com/smartystreets/assertions v0.0.0-20190401211740-f487f9de1cd3 // indirect
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	google.golang.org/protobuf v1.27.1 // indirect
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/code-generator v0.20.2
	k8s.io/klog v0.3.0
)

replace (
	github.com/ulikunitz/xz v0.5.6 => github.com/ulikunitz/xz v0.5.10
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.2.8
)
