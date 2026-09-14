package collect

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	backoffBase = time.Millisecond
	t.Cleanup(func() { backoffBase = time.Second })
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestRetryApenasEm5xx(t *testing.T) {
	t.Run("repete em 500", func(t *testing.T) {
		var calls int
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{}`))
		})
		c := NewClient(srv.URL, "teste", 0)
		if _, err := c.Get(context.Background(), "/x", nil); err != nil {
			t.Fatal(err)
		}
		if calls != 2 || c.Requests() != 2 {
			t.Errorf("%d chamadas e %d contadas, esperava 2", calls, c.Requests())
		}
	})

	// 4xx é bug do worker ou credencial recusada: insistir só gera carga no
	// site alheio.
	t.Run("não repete em 404", func(t *testing.T) {
		var calls int
		srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusNotFound)
		})
		c := NewClient(srv.URL, "teste", 0)
		_, err := c.Get(context.Background(), "/inexistente", nil)
		if !IsStatus(err, http.StatusNotFound) {
			t.Fatalf("esperava *StatusError 404, veio %v", err)
		}
		if calls != 1 {
			t.Errorf("%d chamadas, esperava 1", calls)
		}
	})
}

func TestDoRepeteCorpoECabecalhos(t *testing.T) {
	var bodies []string
	var ua, custom, accept string
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		ua, custom, accept = r.UserAgent(), r.Header.Get("OrganizationId"), r.Header.Get("Accept")
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte(`ok`))
	})

	c := NewClient("https://nao-usado.invalid", "precos-worker/teste", 0)
	h := http.Header{}
	h.Set("OrganizationId", "205")
	// URL absoluta ignora a base: workers falam com mais de um host.
	got, err := c.Do(context.Background(), http.MethodPost, srv.URL+"/login", h, []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok" {
		t.Errorf("corpo = %q", got)
	}
	if len(bodies) != 2 || bodies[1] != `{"a":1}` {
		t.Errorf("corpos recebidos = %q: a repetição precisa reenviar o corpo inteiro", bodies)
	}
	if ua != "precos-worker/teste" || custom != "205" || accept != "application/json" {
		t.Errorf("cabeçalhos: UA=%q OrganizationId=%q Accept=%q", ua, custom, accept)
	}
}

func TestErroNaoExpoeQuery(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	c := NewClient(srv.URL, "teste", 0)
	_, err := c.Get(context.Background(), "/p?token=segredo", nil)
	if err == nil {
		t.Fatal("esperava erro")
	}
	if got := err.Error(); contains(got, "segredo") {
		t.Errorf("mensagem de erro expõe a query: %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
