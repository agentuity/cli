package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type APIClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewAPIClient(baseURL, token string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		token:   token,
		client:  http.DefaultClient,
	}
}

func (c *APIClient) DoWithResponse(method, path string, payload interface{}, response interface{}) error {
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

	if method == "GET" && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request failed with status (%s)", resp.Status)
	} else if method != "GET" && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("request failed with status (%s)", resp.Status)
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("error decoding response: %w", err)
		}
	}
	return nil
}