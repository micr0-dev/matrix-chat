package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"golang.org/x/exp/rand"
)

// OllamaMessage, OllamaRequest, and OllamaResponse for interacting with the API
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  OllamaOptions   `json:"options,omitempty"`
}

type OllamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopK        int     `json:"top_k,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	Seed        int     `json:"seed,omitempty"`
}

type OllamaResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// Function to query Ollama LLM using web API with message history
func queryOllama(conversation []OllamaMessage, systemPrompt string, temperature float64, topK int, topP float64) (string, error) {
	// Build the message history based on the conversation

	// Add the system prompt to the conversation
	messages := []OllamaMessage{
		{Role: "system", Content: systemPrompt},
	}

	// Add the conversation messages to the history
	messages = append(messages, conversation...)

	// Prepare the request payload with tuning options
	requestData := OllamaRequest{
		Model:    config.LLM.Model,
		Messages: messages,
		Stream:   false, // Disable streaming
		Options: OllamaOptions{
			Temperature: temperature,
			TopK:        topK,
			TopP:        topP,
			Seed:        rand.Intn(99999999),
		},
	}

	// Marshal requestData to JSON
	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return "", err
	}

	// Send the request to the Ollama API
	resp, err := http.Post("http://localhost:11434/api/chat", "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Parse the response
	var ollamaResp OllamaResponse
	err = json.Unmarshal(body, &ollamaResp)
	if err != nil {
		return "", err
	}

	return ollamaResp.Message.Content, nil
}
