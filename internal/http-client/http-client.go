package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"

	"github.com/xochilpili/subtitler-cli/internal/logger"
)

type HttpClient interface {
	Get(ctx context.Context, url string, target interface{}) error
	Post(ctx context.Context, url string, body io.Reader, target interface{}, contentType string) error
	DownloadFile(ctx context.Context, url string, fileName string) (string, error)
}

type httpClient struct {
	Timeout time.Duration
	Debug   bool
}

func New(debug bool) *httpClient {
	return &httpClient{
		Timeout: time.Duration(10 * time.Second),
		Debug:   debug,
	}
}

func (h *httpClient) Get(ctx context.Context, url string, target interface{}) error {
	client := &http.Client{Timeout: h.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		panic(fmt.Errorf("error while create a new request: %v", err))
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Add("X-Requested-With", "XMLHttpRequest")
	if h.Debug {
		logger.Debug("%v: %s", "Request to", url)
	}

	resp, err := client.Do(req)

	if err != nil {
		panic(fmt.Errorf("error while requesting data to %s, error: %v", url, err))
	}
	defer resp.Body.Close()

	if h.Debug {
		respDump, _ := httputil.DumpResponse(resp, true)
		logger.Debug("%v:\n%s", "RESPONSE", respDump)
	}

	err = json.NewDecoder(resp.Body).Decode(&target)
	if err != nil {
		return err
	}

	return nil
}

func (h *httpClient) Post(ctx context.Context, url string, body io.Reader, target interface{}, contentType string) error {
	client := &http.Client{Timeout: h.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		panic(fmt.Errorf("error while create a new request: %s", err))
	}

	if contentType != "" && contentType == "form" {
		req.Header.Add("Content-type", "application/x-www-form-urlencoded; charset=UTF-8")
	} else {
		req.Header.Add("Content-type", "application/json")
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	// subdivx dev, you're an idiot.
	req.Header.Add("Cookie", "sdx=cgmbnkmlknnu390mcm6qhas6ht")

	if h.Debug {
		logger.Debug("%v: %s, payload: %v", "Request to", url, body)
	}

	resp, err := client.Do(req)
	if err != nil {
		panic(fmt.Errorf("error requesting data to: %s, payload: %v, err: %w", url, body, err))
	}
	defer resp.Body.Close()
	if h.Debug {
		respDump, _ := httputil.DumpResponse(resp, true)
		logger.Debug("%v:\n%s", "RESPONSE", respDump)
	}

	error := json.NewDecoder(resp.Body).Decode(&target)
	if error != nil {
		return err
	}

	return nil
}

func (h *httpClient) DownloadFile(ctx context.Context, url string, filename string) (string, error) {
	client := &http.Client{
		Timeout: time.Duration(time.Second * 30),
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		panic(fmt.Errorf("unable to create request: %v", err))
	}

	req.Header.Add("Host", "subdivx.com")
	req.Header.Add("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:109.0) Gecko/20100101 Firefox/110.0")
	req.Header.Add("Accept-Language", "en-US,en;q=0.5")
	req.Header.Add("Referer", "https://subdivx.com/descargar.php")
	req.Header.Add("Cookie", "all required cookies")
	req.Header.Add("Connection", "keep-alive")

	if h.Debug {
		logger.Debug("%v: %s", "Request to", url)
	}

	resp, err := client.Do(req)

	if err != nil {
		panic(fmt.Errorf("unable to request %s, error : %v", url, err))
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
		redirectUrl, err := resp.Location()
		if h.Debug {
			logger.Debug("%v: %s", "Redirected to", redirectUrl)
		}

		if err != nil {
			panic(fmt.Errorf("error redirecting to %s, err: %v", redirectUrl, err))
		}

		extensionFile := filepath.Ext(redirectUrl.Path)
		fileName := filename + extensionFile
		file, err := os.Create(fileName)

		if err != nil {
			panic(fmt.Errorf("error creating file: %v", err))
		}

		defer file.Close()

		req.URL = redirectUrl
		resp, err := client.Do(req)
		if err != nil {
			panic(fmt.Errorf("error while sending redirect to %s, error: %v", redirectUrl, err))
		}
		defer resp.Body.Close()

		if h.Debug {
			logger.Debug("%s:\n %v", "REDIRECT REQUEST", req)
		}

		size, err := io.Copy(file, resp.Body)

		if err != nil {
			panic(fmt.Errorf("error while created downloaded file %v", err))
		}

		if h.Debug {
			logger.Debug("%s: %d", "downloaded file", size)
		}
		return fileName, nil
	}
	return "", nil
}
