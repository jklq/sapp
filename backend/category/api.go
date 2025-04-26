package category

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog" // Import slog
	"net/http"
)

type ModelAPIResponse struct {
	Id       string `json:"id"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Object   string `json:"object"`
	Created  int    `json:"created"`
	Choices  []Choice
}

type ModelAPI interface {
	Prompt(prompt string) (*ModelAPIResponse, error)
}

type OpenRouterAPI struct {
	apiKey string
	model  string
}

func NewOpenRouterAPI(apiKey string, model string) OpenRouterAPI {
	if apiKey == "" {
		slog.Warn("OpenRouter API key is empty. Ensure OPENROUTER_KEY environment variable is set.")
	} else {
		// Log length for confirmation, not the key itself for security
		slog.Info("OpenRouter API key loaded.", "key_length", len(apiKey))
	}
	return OpenRouterAPI{apiKey: apiKey, model: model}
}

func (or OpenRouterAPI) Prompt(prompt string) (*ModelAPIResponse, error) {
	// Create the request payload
	payload := ChatCompletionRequest{
		Model: or.model,
		Messages: []Message{
			{
				Role:    "system",
				Content: "Du er norsk og tenker på norsk.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
	}

	// Marshal the payload into JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %v", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set the headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+or.apiKey)

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Check if the response status code is not 200 OK
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status code %d: %s", resp.StatusCode, string(body))
	}

	var unmarshalledRespone = &ModelAPIResponse{}

	err = json.Unmarshal(body, unmarshalledRespone)

	// Return the response body as a string
	return unmarshalledRespone, err
}
