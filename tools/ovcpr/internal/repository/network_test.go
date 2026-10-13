// SPDX-License-Identifier: GPL-3.0-or-later

package repository

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadRetryBoundary(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(fmt.Sprint(interrupted), func(t *testing.T) {
			var requests atomic.Int32
			data := strings.Repeat("x", 128)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := requests.Add(1)
				if interrupted {
					w.Header().Set("Content-Length", fmt.Sprint(len(data)))
					if attempt == 1 {
						fmt.Fprint(w, data[:32])
						return
					}
				} else {
					w.(http.Flusher).Flush()
				}
				fmt.Fprint(w, data)
			}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			policy := DefaultPolicy()
			policy.AllowedPorts = []string{u.Port()}
			if !interrupted {
				policy.MaxDownloadBytes = 64
			}
			root := t.TempDir()
			file, _, err := policy.Download(context.Background(), server.Client(), server.URL+"/archive", root)
			if interrupted {
				if err != nil || requests.Load() != 2 {
					t.Fatal("A transient body read failure was not retried.", requests.Load(), err)
				}
				name := file.Name()
				file.Close()
				contents, err := os.ReadFile(name)
				if err != nil || string(contents) != data {
					t.Fatal("The retry result differs from the response body.", err)
				}
				os.Remove(name)
			} else {
				var resourceErr *Error
				if file != nil || !errors.As(err, &resourceErr) || resourceErr.Code != "RESOURCE_LIMIT_EXCEEDED" || requests.Load() != 1 {
					t.Fatal("A permanent resource failure was retried or misclassified.", requests.Load(), err)
				}
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 0 {
				t.Fatal("Temporary download files remain.", entries, err)
			}
		})
	}
}
