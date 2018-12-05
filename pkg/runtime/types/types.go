package types

// ObjectMeta subset of k8s' ObjectMeta
type ObjectMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Function is mirror of github.com/refunc/refunc/pkg/apis/refunc/v1beta3.Funcdef,
// to avoid the heavy deps on k8s stuffs
type Function struct {
	ObjectMeta `json:"metadata"`

	Spec struct {
		// storage path for function
		Body string `json:"body,omitempty"`
		// unique hash that can identify current function
		Hash string `json:"hash"`
		// The entry name to execute when a function is activated
		Entry string `json:"entry,omitempty"`
		// the maximum number of cocurrent
		MaxReplicas int32 `json:"maxReplicas"`
		// Runtime options for agent and runtime builder
		Runtime struct {
			// name of builder, empty if using default
			Name string `json:"name,omitempty"`

			Envs    map[string]string `json:"envs,omitempty"`
			Timeout int               `json:"timeout,omitempty"`

			Credentials struct {
				AccessKey string `json:"accessKey,omitempty"`
				SecretKey string `json:"secretKey,omitempty"`
				Token     string `json:"token,omitempty"`
			} `json:"credentials"`

			Permissions struct {
				Scope     string   `json:"scope,omitempty"`
				Publish   []string `json:"publish,omitempty"`
				Subscribe []string `json:"subscribe,omitempty"`
			} `json:"permissions"`
		} `json:"runtime"`

		Cmd []string `json:"-"` // parsed entry, for internal use
	} `json:"spec"`
}

// ARN returns Amazon Resource Names
func (fn *Function) ARN() string {
	return "arn:aws:lambda:us-east-1:" + fn.Namespace + ":function:" + fn.Name
}
