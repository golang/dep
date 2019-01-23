// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitbucketapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	tvAuthToken  = "abc123"
	tvAuthHeader = "Bearer " + tvAuthToken
)

func Test_packageInitClient(t *testing.T) {
	defaultClient = nil
	odao := defaultAPIOrigin
	defaultAPIOrigin = "some value"

	defer func() {
		defaultAPIOrigin = odao
		defaultClient = nil
		_ = os.Unsetenv(environmentVariable)
	}()

	tests := []struct {
		name  string
		token string
	}{
		{
			name: "no_token",
		},
		{
			name:  "token",
			token: tvAuthToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				defaultClient = nil
			}()

			_ = os.Setenv(environmentVariable, tt.token)

			packageInitClient()

			if defaultClient == nil {
				t.Fatal("defaultClient = <nil>, want non-nil")
			}

			if defaultClient.token != tt.token {
				t.Fatalf("defaultClient.token = %q, want %q", defaultClient.token, tt.token)
			}

			if defaultClient.endpoint != defaultAPIOrigin {
				t.Fatalf("defaultClient.endpoint = %q, want %q", defaultClient.endpoint, defaultAPIOrigin)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name  string
		httpc *http.Client
		token string
	}{
		{
			name: "no_client_no_token",
		},
		{
			name:  "no_client_token",
			token: "abc123",
		},
		{
			name:  "client_token",
			httpc: pooledClient(),
			token: tvAuthToken,
		},
	}

	var c *Client

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c = New(tt.httpc, tt.token)

			if c.httpc == nil {
				t.Error("c.httpc = <nil>, want non-nil")
			}

			if tt.httpc != nil && c.httpc != tt.httpc {
				t.Error("New() did not appear to set the HTTP client properly")
			}

			if c.token != tt.token {
				t.Errorf("c.token = %q, want %q", c.token, tt.token)
			}
		})
	}
}

func TestClient_do(t *testing.T) {
	var expectAuthHeader bool
	var repo string

	https := httptest.NewServer(testMux(&expectAuthHeader, &repo))

	c := &Client{
		httpc: pooledTestClient(),
	}

	tests := []struct {
		name   string
		urlStr string
		token  string
		body   []byte
		err    string
	}{
		{
			name:   "connection_error",
			urlStr: "http://127.0.1.2",
			err:    "Get http://127.0.1.2: dial tcp 127.0.1.2:80:",
		},
		{
			name:   "happy_path_no_token",
			urlStr: https.URL + "/repositories/nothing/here",
			body:   []byte(`{"scm":"hg"}`),
		},
		{
			name:   "happy_path_token",
			urlStr: https.URL + "/repositories/nothing/here",
			token:  tvAuthToken,
			body:   []byte(`{"scm":"hg"}`),
		},
	}

	for _, tt := range tests {
		// implicit type assertions
		var req *http.Request
		var resp *http.Response
		var err error

		t.Run(tt.name, func(t *testing.T) {
			c.token = tt.token

			expectAuthHeader = len(tt.token) > 0

			req, err = http.NewRequest(http.MethodGet, tt.urlStr, nil)
			if err != nil {
				t.Fatalf("unexpected error building request: %v", err)
			}

			resp, err = c.do(req)
			if err != nil {
				if len(tt.err) > 0 {
					if strings.Contains(err.Error(), tt.err) {
						return
					}
					t.Fatalf("did not find %q in error %q", tt.err, err)
				}
				t.Fatalf("c.do() unexpected error: %s", err)
			}

			defer func() {
				_, _ = io.Copy(ioutil.Discard, resp.Body)
				_ = resp.Body.Close
			}()

			if len(tt.err) > 0 {
				t.Fatalf("c.do() error %q did not occur as expected", tt.err)
			}

			body, _ := ioutil.ReadAll(resp.Body)
			if !bytes.Equal(body, tt.body) {
				t.Fatalf("body = %q, want %q", body, tt.body)
			}
		})
	}
}

func TestClient_get(t *testing.T) {
	var expectAuthHeader bool
	var repo string

	expiredCtx, cancel := context.WithCancel(context.Background())
	cancel()

	https := httptest.NewServer(testMux(&expectAuthHeader, &repo))

	c := &Client{
		httpc: pooledTestClient(),
	}

	tests := []struct {
		name   string
		urlStr string
		ctx    context.Context
		token  string
		body   []byte
		err    string
	}{
		{
			name:   "invalid_URL",
			urlStr: `://\\!~`,
			ctx:    context.Background(),
			err:    "missing protocol scheme",
		},
		{
			name:   "connection_error",
			urlStr: "http://127.0.1.2",
			ctx:    context.Background(),
			err:    "Get http://127.0.1.2: dial tcp 127.0.1.2:80:",
		},
		{
			name:   "bad_status_code",
			urlStr: https.URL + "/bsc/repositories/nothing/here",
			ctx:    context.Background(),
			err:    fmt.Sprintf(`unexpected HTTP status for GET "%s/bsc/repositories/nothing/here": 420 status code 420`, https.URL),
		},
		{
			name:   "happy_path_expired_context",
			urlStr: https.URL + "/repositories/nothing/here",
			ctx:    expiredCtx,
			err:    fmt.Sprintf("Get %s/repositories/nothing/here: context canceled", https.URL),
		},
		{
			name:   "happy_path_no_token",
			urlStr: https.URL + "/repositories/nothing/here",
			ctx:    context.Background(),
			body:   []byte(`{"scm":"hg"}`),
		},
		{
			name:   "happy_path_token",
			urlStr: https.URL + "/repositories/nothing/here",
			ctx:    context.Background(),
			token:  tvAuthToken,
			body:   []byte(`{"scm":"hg"}`),
		},
	}

	for _, tt := range tests {
		// implicit type assertions
		var body []byte
		var err error

		t.Run(tt.name, func(t *testing.T) {
			c.token = tt.token

			expectAuthHeader = len(tt.token) > 0

			body, err = c.get(tt.ctx, tt.urlStr)
			if err != nil {
				if len(tt.err) > 0 {
					if strings.Contains(err.Error(), tt.err) {
						return
					}
					t.Fatalf("did not find %q in error %q", tt.err, err)
				}
				t.Fatalf("c.get() unexpected error: %s", err)
			}

			if len(tt.err) > 0 {
				t.Fatalf("c.get() error %q did not occur as expected", tt.err)
			}

			if !bytes.Equal(body, tt.body) {
				t.Fatalf("body = %q, want %q", body, tt.body)
			}
		})
	}
}

func TestClient_GetSCM(t *testing.T) {
	var expectAuthHeader bool
	var repo string

	expiredCtx, cancel := context.WithCancel(context.Background())
	cancel()

	https := httptest.NewServer(testMux(&expectAuthHeader, &repo))

	c := &Client{
		httpc: pooledTestClient(),
	}

	tests := []struct {
		name    string
		baseURL string
		token   string
		project string
		ctx     context.Context
		scm     string
		err     string
	}{
		{
			name:    "get_error",
			baseURL: "http://127.0.1.2/",
			project: "dep/test",
			ctx:     context.Background(),
			err:     `failed to GET "http://127.0.1.2//repositories/dep/test": Get http://127.0.1.2//repositories/dep/test: dial tcp 127.0.1.2:80:`,
		},
		{
			name:    "bad_json",
			baseURL: https.URL + "/json",
			project: "dep/test",
			ctx:     context.Background(),
			err:     `failed to unmarshal response body JSON: unexpected end of JSON input`,
		},
		{
			name:    "happy_path_expired_context",
			baseURL: https.URL,
			project: "dep/test",
			ctx:     expiredCtx,
			err:     fmt.Sprintf("Get %s/repositories/dep/test: context canceled", https.URL),
		},
		{
			name:    "happy_path",
			baseURL: https.URL,
			project: "dep/test",
			ctx:     context.Background(),
			scm:     "hg",
		},
	}

	for _, tt := range tests {
		var scm string
		var err error

		t.Run(tt.name, func(t *testing.T) {
			expectAuthHeader = len(tt.token) > 0
			repo = tt.project

			c.endpoint = tt.baseURL
			c.token = tt.token

			scm, err = c.GetSCM(tt.ctx, tt.project)
			if err != nil {
				if len(tt.err) > 0 {
					if strings.Contains(err.Error(), tt.err) {
						return
					}
					t.Fatalf("did not find %q in error %q", tt.err, err)
				}
				t.Fatalf("c.GetSCM() unexpected error: %s", err)
			}

			if len(tt.err) > 0 {
				t.Fatalf("c.GetSCM() error %q did not occur as expected", tt.err)
			}

			if scm != tt.scm {
				t.Fatalf("scm = %q, want %q", scm, tt.scm)
			}
		})
	}
}

func TestGetSCM(t *testing.T) {
	defaultClient = nil

	var expectAuthHeader bool
	var repo string

	expiredCtx, cancel := context.WithCancel(context.Background())
	cancel()

	https := httptest.NewServer(testMux(&expectAuthHeader, &repo))

	odao := defaultAPIOrigin
	defaultAPIOrigin = https.URL

	defer func() {
		defaultAPIOrigin = odao
		defaultClient = nil
	}()

	tests := []struct {
		name    string
		baseURL string
		token   string
		project string
		ctx     context.Context
		scm     string
		err     string
	}{
		{
			name:    "happy_path",
			baseURL: https.URL,
			project: "dep/test",
			ctx:     context.Background(),
			scm:     "hg",
		},
		{
			name:    "happy_path_expired_context",
			baseURL: https.URL,
			project: "dep/test",
			ctx:     expiredCtx,
			err:     fmt.Sprintf("Get %s/repositories/dep/test: context canceled", https.URL),
		},
		{
			name:    "get_error",
			baseURL: "http://127.0.1.2",
			project: "dep/test",
			ctx:     context.Background(),
			err:     `failed to GET "http://127.0.1.2/repositories/dep/test": Get http://127.0.1.2/repositories/dep/test: dial tcp 127.0.1.2:80:`,
		},
		{
			name:    "bad_json",
			baseURL: https.URL + "/json",
			project: "dep/test",
			ctx:     context.Background(),
			err:     `failed to unmarshal response body JSON: unexpected end of JSON input`,
		},
	}

	for _, tt := range tests {
		var scm string
		var err error

		t.Run(tt.name, func(t *testing.T) {
			expectAuthHeader = len(tt.token) > 0
			repo = tt.project

			if defaultClient != nil {
				defaultClient.httpc = pooledTestClient()
				defaultClient.endpoint = tt.baseURL
				defaultClient.token = tt.token
			}

			scm, err = GetSCM(tt.ctx, tt.project)
			if err != nil {
				if len(tt.err) > 0 {
					if strings.Contains(err.Error(), tt.err) {
						return
					}
					t.Fatalf("did not find %q in error %q", tt.err, err)
				}
				t.Fatalf("GetSCM() unexpected error: %s", err)
			}

			if len(tt.err) > 0 {
				t.Fatalf("GetSCM() error %q did not occur as expected", tt.err)
			}

			if defaultClient == nil {
				t.Fatalf("defaultClient should not be nil")
			}

			if scm != tt.scm {
				t.Fatalf("scm = %q, want %q", scm, tt.scm)
			}
		})
	}
}

func testMux(expectAuthHeader *bool, repo *string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/noop/repositories/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "noop")
	})

	mux.HandleFunc("/bsc/repositories/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(420) // enhance your calm
		fmt.Fprint(w, "bad status code: 420")
	})

	mux.HandleFunc("/json/repositories/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{")
	})

	mux.HandleFunc("/repositories/", func(w http.ResponseWriter, r *http.Request) {
		if *expectAuthHeader {
			auth := r.Header.Get("Authorization")
			if auth != tvAuthHeader {
				w.WriteHeader(http.StatusUnauthorized)
				errstr := fmt.Sprintf("Authorization header = %q, want %q", auth, tvAuthHeader)
				fmt.Fprint(w, errstr)
				log.Print(errstr)
				return
			}
		}

		if len(*repo) > 0 {
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) < 3 {
				w.WriteHeader(http.StatusNotFound)
				errstr := fmt.Sprintf("invalid path %q, want '/repositories/<user>/<repo>'", r.URL.Path)
				fmt.Fprint(w, errstr)
				log.Print(errstr)
				return
			}

			reqRepo := parts[len(parts)-2] + "/" + parts[len(parts)-1]
			if reqRepo != *repo {
				w.WriteHeader(http.StatusBadRequest)
				errstr := fmt.Sprintf("invalid repo %q, want %q", reqRepo, *repo)
				fmt.Fprint(w, errstr)
				log.Print(errstr)
				return
			}
		}

		fmt.Fprint(w, `{"scm":"hg"}`)
	})

	return mux
}

func pooledTestClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   100 * time.Millisecond,
				KeepAlive: 5 * time.Second,
				DualStack: true,
			}).DialContext,
			IdleConnTimeout:       10 * time.Second,
			ExpectContinueTimeout: 100 * time.Millisecond,
			MaxIdleConns:          2,
			MaxIdleConnsPerHost:   runtime.GOMAXPROCS(0) + 1,
			Proxy:                 http.ProxyFromEnvironment,
		},
	}
}
