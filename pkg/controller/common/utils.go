package common

import (
	"bytes"
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

// RestResult is a struct for REST call result
type RestResult struct {
	StatusCode int
	Body       string
	ErrorType  string
}

// restCallFunc : common rest call fun
func RestCallFunc(rsString string, postBody []byte, method string, header string, token string, expectReturn bool) (RestResult, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	restClient := http.Client{
		Timeout:   time.Second * 300,
		Transport: tr,
	}
	u, _ := url.ParseRequestURI(rsString)
	urlStr := u.String()
	var req *http.Request
	if postBody != nil {

		req, _ = http.NewRequest(method, urlStr, bytes.NewBuffer(postBody))
	} else {
		req, _ = http.NewRequest(method, urlStr, nil)
	}

	if token != "" {
		if header == "" {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set(header, token)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := restClient.Do(req)
	if err != nil {
		return RestResult{}, err
	}
	defer res.Body.Close()

	if expectReturn {
		body, err := ioutil.ReadAll(res.Body)
		result := RestResult{StatusCode: res.StatusCode, Body: string(body[:])}
		return result, err
	}
	return RestResult{}, nil
}
