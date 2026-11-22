package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCheckEnrollmentServer(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/system/readiness" || r.Header.Get("Authorization") != "" {
					t.Error("preflight must only check readiness, without sending a credential")
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"status":"ready"}`))
			}))
			origin, _ := url.Parse(server.URL)
			err := checkEnrollmentServer(context.Background(), server.Client(), origin)
			if (err == nil) != (status == http.StatusOK) {
				t.Fatalf("readiness result: %v", err)
			}
			server.Close()
			err = checkEnrollmentServer(context.Background(), server.Client(), origin)
			if err == nil || !strings.Contains(err.Error(), "LAN HTTPS address") {
				t.Fatalf("missing loopback troubleshooting: %v", err)
			}
		})
	}
}
