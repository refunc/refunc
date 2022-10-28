package httptrigger

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

// https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-develop-integrations-lambda.html#http-api-develop-integrations-lambda.proxy-format

type RequestPayload struct {
	Version               string            `json:"version"`
	RawPath               string            `json:"rawPath"`
	RawQueryString        string            `json:"rawQueryString"`
	Cookies               []string          `json:"cookies"`
	Headers               map[string]string `json:"headers"`
	QueryStringParameters map[string]string `json:"queryStringParameters"`
	PathParameters        map[string]string `json:"pathParameters"`
	IsBase64Encoded       bool              `json:"isBase64Encoded"`
	Body                  string            `json:"body"`
	Context               RequestContext    `json:"requestContext"`
}

type RequestContext struct {
	DomainName string             `json:"domainName"`
	HTTP       RequestContextHTTP `json:"http"`
	RequestID  string             `json:"requestId"`
}

type RequestContextHTTP struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	Protocol  string `json:"protocol"`
	SourceIP  string `json:"sourceIp"`
	UserAgent string `json:"userAgent"`
}

type ResponsePayload struct {
	StatusCode      int               `json:"statusCode"`
	Headers         map[string]string `json:"headers"`
	Body            string            `json:"body"`
	Cookies         []string          `json:"cookies"`
	IsBase64Encoded bool              `json:"isBase64Encoded"`
}

func formatRequestPayload(req *http.Request) (RequestPayload, error) {
	queryStringParameters := map[string]string{}
	for k, v := range req.URL.Query() {
		queryStringParameters[k] = strings.Join(v, ",")
	}

	headers := map[string]string{}
	for k, v := range req.Header {
		headers[k] = strings.Join(v, ",")
	}

	cookies := []string{}
	for _, cookie := range req.Header["Cookie"] {
		for _, v := range strings.Split(cookie, ";") {
			cookies = append(cookies, strings.TrimSpace(v))
		}
	}

	payload := RequestPayload{
		Version:               "2.0",
		RawPath:               req.URL.EscapedPath(),
		RawQueryString:        req.URL.RawQuery,
		Cookies:               cookies,
		Headers:               headers,
		QueryStringParameters: queryStringParameters,
		PathParameters:        map[string]string{},
		Context: RequestContext{
			DomainName: req.Host,
			HTTP: RequestContextHTTP{
				Method:    req.Method,
				Path:      req.URL.Path,
				Protocol:  req.Proto,
				SourceIP:  req.RemoteAddr,
				UserAgent: req.UserAgent(),
			},
			RequestID: getRequestID(req),
		},
	}

	bts, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return payload, err
	}
	if isBinary(bts) {
		payload.Body = base64.StdEncoding.EncodeToString(bts)
		payload.IsBase64Encoded = true
	} else {
		payload.Body = string(bts)
		payload.IsBase64Encoded = false
	}

	return payload, nil
}

func isBinary(bts []byte) bool {
	isBinary := true
	for mtype := mimetype.Detect(bts); mtype != nil; mtype = mtype.Parent() {
		if mtype.Is("text/plain") {
			isBinary = false
		}
	}
	return isBinary
}

func formatResponsePayload(bts []byte) (ResponsePayload, error) {
	payload := ResponsePayload{StatusCode: 0}
	// is valid JSON
	if err := json.Unmarshal(bts, &payload); err != nil {
		return payload, err
	}
	// have status code
	if payload.StatusCode == 0 {
		payload.StatusCode = 200
		payload.Headers = map[string]string{
			"Content-Type": jsonCT,
		}
		payload.IsBase64Encoded = false
		payload.Body = string(bts)
	}
	// customize
	return payload, nil
}
