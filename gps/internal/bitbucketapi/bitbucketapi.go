// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bitbucketapi provides an extremely minimal Bitbucket API client. It
// looks for a BITBUCKET_API_TOKEN environment variable to use authentication,
// otherwise it relies on anonymous API access.
package bitbucketapi

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/pkg/errors"
)

type repository struct {
	SCM string `json:"scm"`
}

const environmentVariable = "BITBUCKET_API_TOKEN"

var (
	defaultClient    *Client
	initOnce         = &sync.Once{}
	defaultAPIOrigin = "https://api.bitbucket.org/2.0" // a var so we can hax it during tests
)

// Client is the API client to Bitbucket.
type Client struct {
	endpoint string
	token    string
	httpc    *http.Client
}

func packageInitClient() {
	token := os.Getenv(environmentVariable)

	defaultClient = New(pooledClient(), token)
}

// New instantiates a new *Client, taking an optional *http.Client and token.
func New(httpClient *http.Client, token string) *Client {
	if httpClient == nil {
		httpClient = pooledClient()
	}

	return &Client{
		endpoint: defaultAPIOrigin,
		token:    token,
		httpc:    httpClient,
	}
}

func (c *Client) do(r *http.Request) (*http.Response, error) {
	if len(c.token) > 0 {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}

	return c.httpc.Do(r)
}

func (c *Client) get(ctx context.Context, urlStr string) ([]byte, error) {
	// build request
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)

	// action request
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, errors.Errorf("unexpected HTTP status for GET %q: %s", urlStr, resp.Status)
	}

	defer func() { _ = resp.Body.Close() }() // make errcheck happy

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	return body, nil
}

// GetSCM takes a project and returns the SCM string for the project: git or hg.
func (c *Client) GetSCM(ctx context.Context, project string) (string, error) {
	urlStr := c.endpoint + "/repositories/" + project

	jsonBody, err := c.get(ctx, urlStr)
	if err != nil {
		return "", errors.Wrapf(err, "failed to GET %q", urlStr)
	}

	var repo repository

	if err := json.Unmarshal(jsonBody, &repo); err != nil {
		return "", errors.Wrap(err, "failed to unmarshal response body JSON")
	}

	return repo.SCM, nil
}

// GetSCM takes a project and returns the SCM string for the project: git or hg.
// Uses the internal singleton client.
func GetSCM(ctx context.Context, project string) (string, error) {
	// lazy load the default API client
	initOnce.Do(packageInitClient)

	return defaultClient.GetSCM(ctx, project)
}

func pooledClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
			Proxy:                 http.ProxyFromEnvironment,
		},
	}
}
