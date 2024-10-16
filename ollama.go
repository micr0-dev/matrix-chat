package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// OllamaRequest and OllamaResponse for interacting with the API
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type OllamaResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// Function to query Ollama LLM using web API with message history
func queryOllama(prompt string, conversation []OllamaMessage, systemPrompt string) (string, error) {
	// Build the message history based on the conversation

	// Add the system prompt to the conversation
	messages := []OllamaMessage{
		{Role: "system", Content: systemPrompt},
	}

	// Add the conversation messages to the history
	messages = append(messages, conversation...)

	// Add the user prompt to the history
	messages = append(messages, OllamaMessage{Role: "user", Content: prompt})

	// Prepare the request payload
	requestData := OllamaRequest{
		Model:    config.LLM.Model,
		Messages: messages,
		Stream:   false, // Disable streaming
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
