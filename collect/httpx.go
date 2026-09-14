package collect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const (
	requestTimeout = 30 * time.Second
	maxAttempts    = 3
)

// backoffBase é a espera da primeira repetição; dobra a cada tentativa.
// Variável para os testes não esperarem segundos.
var backoffBase = time.Second

// StatusError é uma resposta 4xx. Não é repetida: 4xx é bug do worker ou
// credencial recusada, e insistir só gera carga no site alheio. O tipo existe
// para quem precisa reagir a um código específico (ex.: 401 → novo login).
type StatusError struct {
	Code   int
	Method string
	URL    string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d (não repetido)", e.Method, e.URL, e.Code)
}

// IsStatus diz se err é uma resposta com aquele código.
func IsStatus(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == code
}

// Client é o cliente HTTP educado dos workers: timeout explícito, User-Agent
// identificável, intervalo entre requisições e retry só onde faz sentido.
//
// Não é seguro para uso concorrente — e não precisa ser: a coleta é sequencial
// de propósito.
type Client struct {
	baseURL   string
	userAgent string
	http      *http.Client
	delay     time.Duration
	last      time.Time
	requests  int
}

// NewClient cria um cliente. baseURL resolve caminhos relativos; URLs absolutas
// passam direto, para workers que falam com mais de um host.
func NewClient(baseURL, userAgent string, delay time.Duration) *Client {
	if delay < 0 {
		delay = 0
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		userAgent: userAgent,
		delay:     delay,
		http: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				Proxy:           http.ProxyFromEnvironment,
				MaxIdleConns:    10,
				IdleConnTimeout: 90 * time.Second,
			},
		},
	}
}

// Requests conta as requisições feitas, incluindo repetições.
func (c *Client) Requests() int { return c.requests }

// Get busca uma URL e devolve o corpo.
func (c *Client) Get(ctx context.Context, target string, header http.Header) ([]byte, error) {
	return c.Do(ctx, http.MethodGet, target, header, nil)
}

// Do executa a requisição com intervalo e retry.
//
// Repete apenas em 5xx e erro de transporte, com backoff exponencial e jitter.
// 4xx devolve *StatusError na primeira tentativa.
func (c *Client) Do(ctx context.Context, method, target string, header http.Header, body []byte) ([]byte, error) {
	u := c.resolve(target)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}

		respBody, status, err := c.do(ctx, method, u, header, body)
		c.requests++

		switch {
		case err == nil && status >= 200 && status < 300:
			return respBody, nil
		case err == nil && status >= 400 && status < 500:
			return nil, &StatusError{Code: status, Method: method, URL: redactQuery(u)}
		case err != nil:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("%s %s: %w", method, redactQuery(u), err)
		default:
			lastErr = fmt.Errorf("%s %s: HTTP %d", method, redactQuery(u), status)
		}

		if attempt < maxAttempts {
			if err := sleepBackoff(ctx, attempt); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("após %d tentativas: %w", maxAttempts, lastErr)
}

func (c *Client) resolve(target string) string {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		return target
	}
	if !strings.HasPrefix(target, "/") {
		target = "/" + target
	}
	return c.baseURL + target
}

// wait aplica o intervalo entre requisições.
func (c *Client) wait(ctx context.Context) error {
	if c.delay == 0 || c.last.IsZero() {
		c.last = time.Now()
		return nil
	}
	if sleep := c.delay - time.Since(c.last); sleep > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
	}
	c.last = time.Now()
	return nil
}

func (c *Client) do(ctx context.Context, method, u string, header http.Header, body []byte) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		// Um reader novo por tentativa: o anterior já foi consumido.
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, 0, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return respBody, resp.StatusCode, nil
}

// redactQuery tira a query string das mensagens de erro: é onde uma URL
// costuma carregar coisa que não deve ir para log.
func redactQuery(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i] + "?…"
	}
	return u
}

// sleepBackoff espera de forma exponencial com jitter.
func sleepBackoff(ctx context.Context, attempt int) error {
	base := backoffBase * time.Duration(1<<uint(attempt-1))
	jitter := time.Duration(rand.Int63n(int64(base/2) + 1))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(base + jitter):
		return nil
	}
}
