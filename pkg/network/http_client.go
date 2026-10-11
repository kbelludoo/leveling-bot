package network

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPClient struct {
	client  *http.Client
	baseURL string
	headers map[string]string
}

func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: timeout,
		},
		headers: map[string]string{
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"Accept":          "application/json, text/plain, */*",
			"Accept-Language": "pt-BR,pt;q=0.9,en-US;q=0.8,en;q=0.7",
		},
	}
}

func (c *HTTPClient) SetHeader(k, v string) {
	c.headers[k] = v
}

func (c *HTTPClient) SetCookie(cookies string) {
	c.headers["Cookie"] = cookies
}

func (c *HTTPClient) PostJSON(endpoint string, reqBody any, respTarget any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequest(http.MethodPost, url, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("requisição falhou (%s): %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d em %s: %s", resp.StatusCode, endpoint, string(respBytes))
	}

	if respTarget != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, respTarget); err != nil {
			return fmt.Errorf("falha ao deserializar resposta: %w", err)
		}
	}

	return nil
}

func (c *HTTPClient) GetJSON(endpoint string, respTarget any) error {
	url := c.baseURL + endpoint
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("requisição falhou (%s): %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d em %s: %s", resp.StatusCode, endpoint, string(respBytes))
	}

	if respTarget != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, respTarget); err != nil {
			return fmt.Errorf("falha ao deserializar resposta: %w", err)
		}
	}

	return nil
}
