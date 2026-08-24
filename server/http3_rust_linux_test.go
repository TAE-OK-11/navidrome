//go:build linux

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAltSvcForAddress(t *testing.T) {
	t.Parallel()

	if got, want := altSvcForAddress("0.0.0.0:4533", 5*time.Minute), `h3=":4533"; ma=300`; got != want {
		t.Fatalf("altSvcForAddress() = %q, want %q", got, want)
	}
	if got := altSvcForAddress("invalid", time.Minute); got != "" {
		t.Fatalf("altSvcForAddress(invalid) = %q, want empty", got)
	}
}

func TestAuthenticatedHTTP3Bridge(t *testing.T) {
	t.Parallel()

	called := false
	handler := authenticatedHTTP3Bridge("secret", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		if req.Header.Get(rustHTTP3TokenHeader) != "" {
			t.Error("private bridge token leaked to application handler")
		}
		if req.TLS == nil || !req.TLS.HandshakeComplete {
			t.Error("outer TLS state was not restored")
		}
		if req.ProtoMajor != 3 || req.Proto != "HTTP/3.0" {
			t.Errorf("protocol=%s/%d, want HTTP/3.0", req.Proto, req.ProtoMajor)
		}
		if req.Host != "music.example" {
			t.Errorf("host=%q, want original authority", req.Host)
		}
		if req.RemoteAddr != "192.0.2.10:54321" {
			t.Errorf("remoteAddr=%q, want original QUIC peer", req.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://navidrome.local/rest/ping", nil)
	// The transport RemoteAddr is not a security boundary anymore: the bridge is
	// an inherited AF_UNIX socketpair with no externally reachable listener.
	req.RemoteAddr = "socketpair"
	req.Header.Set(rustHTTP3TokenHeader, "secret")
	req.Header.Set(rustHTTP3AuthorityHeader, "music.example")
	req.Header.Set(rustHTTP3RemoteAddrHeader, "192.0.2.10:54321")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || !called {
		t.Fatalf("authorized bridge status=%d called=%v", res.Code, called)
	}
}

func TestAuthenticatedHTTP3BridgeRejectsUntrustedRequests(t *testing.T) {
	t.Parallel()

	handler := authenticatedHTTP3Bridge("secret", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("untrusted request reached application handler")
	}))
	for _, test := range []struct {
		name           string
		token          string
		outerRemote    string
		expectedStatus int
	}{
		{name: "wrong token", token: "wrong", expectedStatus: http.StatusForbidden},
		{name: "missing token", token: "", expectedStatus: http.StatusForbidden},
		{name: "invalid original peer", token: "secret", outerRemote: "invalid", expectedStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://navidrome.local/", nil)
			req.RemoteAddr = "socketpair"
			if test.token != "" {
				req.Header.Set(rustHTTP3TokenHeader, test.token)
			}
			if test.outerRemote != "" {
				req.Header.Set(rustHTTP3RemoteAddrHeader, test.outerRemote)
			}
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != test.expectedStatus {
				t.Fatalf("status=%d, want %d", res.Code, test.expectedStatus)
			}
		})
	}
}

func BenchmarkAuthenticatedHTTP3Bridge(b *testing.B) {
	handler := authenticatedHTTP3Bridge("secret", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "http://navidrome.local/rest/ping", nil)
	req.RemoteAddr = "socketpair"
	res := httptest.NewRecorder()
	token := []string{"secret"}
	authority := []string{"music.example"}
	remoteAddr := []string{"192.0.2.10:54321"}
	b.ReportAllocs()

	for b.Loop() {
		req.Header[rustHTTP3TokenHeader] = token
		req.Header[rustHTTP3AuthorityHeader] = authority
		req.Header[rustHTTP3RemoteAddrHeader] = remoteAddr
		handler.ServeHTTP(res, req)
	}
}

func TestClearHTTP3Advertisement(t *testing.T) {
	t.Parallel()

	handler := clearHTTP3Advertisement(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "https://music.example/ping", nil))
	if got := res.Header().Get("Alt-Svc"); got != "clear" {
		t.Fatalf("Alt-Svc=%q, want clear", got)
	}
}
