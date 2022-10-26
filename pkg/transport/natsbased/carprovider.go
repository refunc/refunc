package natsbased

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/transport"
	"github.com/refunc/refunc/pkg/version"
)

type carproider struct {
}

func (*carproider) Name() string { return "nats" }

func (*carproider) GetTransportContainer(tpl *rfv1beta3.Xenv) *corev1.Container {
	var extraCfg struct {
		Sidecar struct {
			Command []string `json:"command,omitempty"`
		} `json:"sidecar,omitempty"`
	}
	json.Unmarshal(tpl.Spec.Extra, &extraCfg) // nolint:errcheck

	container := defaultCarContainer.DeepCopy()
	if len(extraCfg.Sidecar.Command) > 0 {
		container.Command = extraCfg.Sidecar.Command
	}
	return container
}

var defaultCarContainer = corev1.Container{
	Name:            "nats-sidecar",
	Image:           sidecarContainerImage(),
	ImagePullPolicy: corev1.PullIfNotPresent,
	Command:         []string{"sidecar", "--v", "3"},
	Resources: corev1.ResourceRequirements{
		// set sidecar limit same to func pod body
		// Limits: corev1.ResourceList{
		// 	corev1.ResourceCPU:    resource.MustParse("1000m"),
		// 	corev1.ResourceMemory: resource.MustParse("1Gi"),
		// },
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10m"),
			corev1.ResourceMemory: resource.MustParse("32Mi"),
		},
	},
}

var (
	// SidecarContainerImage default init container image
	// the InitContainerImage can be override by env REFUNC_SIDECAR_IMAGE
	SidecarContainerImage = "refunc/sidecar:$latest"
	iciCheckEnvOnce       sync.Once // init container check

	historyLimits int32 = 3
)

func sidecarContainerImage() string {
	iciCheckEnvOnce.Do(func() {
		if ci := os.Getenv("REFUNC_SIDECAR_IMAGE"); ci != "" {
			SidecarContainerImage = ci
		}
		SidecarContainerImage = strings.Replace(SidecarContainerImage, "$latest", version.SidecarVersion, -1)
	})
	return SidecarContainerImage
}

func init() {
	transport.Register(new(carproider))
}
