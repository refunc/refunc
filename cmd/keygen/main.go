package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	jwt "github.com/golang-jwt/jwt"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

var config struct {
	signToken    bool
	exp          time.Duration
	genConfigMap bool
	ns, name     string
	scope        string
}

func init() {
	flag.BoolVar(&config.signToken, "sign", false, "sign token read from stdin")
	flag.DurationVar(&config.exp, "exp", 0, "exp for token")
	flag.BoolVar(&config.genConfigMap, "configmap", false, "output config map JSON")
	flag.StringVar(&config.ns, "namespace", os.Getenv("REFUNC_NAMESPACE"), "namespace for configmap")
	flag.StringVar(&config.name, "name", "", "namespace for configmap")
	flag.StringVar(&config.scope, "scope", "", "scope for credentials")
}

func main() {
	flag.Parse()

	if config.signToken {
		bts, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			fatal("failed to read token JSON, %v", err)
		}
		var claims jwt.MapClaims
		if err := json.Unmarshal(bts, &claims); err != nil {
			fatal("failed to parse token, %v", err)
		}
		if config.exp > 0 {
			claims["exp"] = jwt.TimeFunc().Add(config.exp).Unix()
		}
		token, err := rfutil.Sign(claims)
		if err != nil {
			fatal("failed to sign token, %v", err)
		}
		fmt.Println(token)
		return
	}

	accessKey, secretKey := env.RandomCredentials()
	if !config.genConfigMap {
		fmt.Println(accessKey)
		fmt.Println(secretKey)
		return
	}

	if config.name == "" {
		if args := flag.Args(); len(args) == 1 {
			config.name = args[0]
		} else {
			fatal("-name must be set")
		}
	}

	var credConfigMap = map[string]interface{}{
		"kind":       "ConfigMap",
		"apiVersion": "v1",
		"metadata": map[string]interface{}{
			"namespace": config.ns,
			"name":      config.name,
			"annotations": map[string]string{
				"refunc.io/is-credential-config": "true",
			},
		},
		"data": map[string]string{
			"accessKey": accessKey,
			"secretKey": secretKey,
			"scope":     config.scope,
		},
	}
	bts, _ := json.Marshal(credConfigMap)
	fmt.Println(string(bts))
}

func fatal(format string, args ...interface{}) {
	log.Println(fmt.Sprintf(format, args...))
	os.Exit(-1)
}
