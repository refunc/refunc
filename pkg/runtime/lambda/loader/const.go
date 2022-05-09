package loader

const (
	LabelLambdaVersion = "lambda.refunc.io/version"
	LabelLambdaName    = "lambda.refunc.io/name"

	AnnotationLambdaConcurrency = "lambda.refunc.io/concurrency"

	MaxLambdaConcurrency = 32
)

var LogFrameDelimer = []byte{165, 90, 0, 1} //0xA55A0001
