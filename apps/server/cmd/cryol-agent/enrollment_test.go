package main

import (
	"context"
	"fmt"
	"io"
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

func TestVisibleEnrollmentInput(t *testing.T) {
	const token = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	for _, input := range []string{token + "\r\n", "  " + token + "  \n"} {
		var prompt strings.Builder
		got, err := promptEnrollmentToken(strings.NewReader(input), &prompt, 0, true)
		if err != nil || strings.TrimSpace(string(got)) != token {
			t.Fatalf("pasted token rejected: %v", err)
		}
		if !strings.Contains(prompt.String(), "visible") || strings.Contains(prompt.String(), token) {
			t.Fatal("the prompt must describe visibility without printing the token itself")
		}
	}
	for _, input := range []string{"not-a-token\n", strings.Repeat("A", 200) + "\n"} {
		if _, err := promptEnrollmentToken(strings.NewReader(input), io.Discard, 0, true); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}

func TestServerOriginForLAN(t *testing.T) {
	for _, value := range []string{
		"https://192.168.1.2:8443",
		"https://cryol.example.internal:8443",
		"https://[fd00::12]:8443",
	} {
		if _, err := parseServerOrigin(value, false); err != nil {
			t.Fatalf("rejected %s: %v", value, err)
		}
	}
	for _, value := range []string{
		"http://192.168.1.2:8443",
		"https://name:secret@cryol.example.internal",
		"https://cryol.example.internal/other-path",
	} {
		if _, err := parseServerOrigin(value, false); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}

func TestEnrollmentIdentifiesUntrustedIssuer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer server.Close()
	origin, _ := url.Parse(server.URL)
	client, err := newClient("")
	if err != nil {
		t.Fatal(err)
	}
	err = checkEnrollmentServer(context.Background(), client, origin)
	if err == nil || !strings.Contains(err.Error(), "issuer is not trusted") {
		t.Fatalf("did not identify the untrusted issuer: %v", err)
	}
}
