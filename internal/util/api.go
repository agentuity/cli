package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/agentuity/go-common/logger"
)

type APIClient struct {
	baseURL string
	token   string
	client  *http.Client
	logger  logger.Logger
}

func NewAPIClient(logger logger.Logger, baseURL, token string) *APIClient {
	return &APIClient{
		logger:  logger,
		baseURL: baseURL,
		token:   token,
		client:  http.DefaultClient,
	}
}

func (c *APIClient) Do(method, path string, payload interface{}, response interface{}) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("error parsing base url: %w", err)
	}
	u.Path = path

	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("error marshalling payload: %w", err)
		}
	}

	c.logger.Debug("request: %s %s", method, u.String())

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	c.logger.Debug("response status: %s", resp.Status)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	c.logger.Debug("response body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && response == nil {
		return fmt.Errorf("request failed with status (%s)", resp.Status)
	}

	if response != nil {
		if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(response); err != nil {
			return fmt.Errorf("error decoding response: %w", err)
		}
	}
	return nil
}
