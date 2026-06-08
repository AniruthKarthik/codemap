package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GenerateResponse struct {
	Purpose           string `json:"purpose"`
	LearningObjective string `json:"learning_objective"`
}

const (
	PromptGenerate = `You are an expert software architect. Analyze the provided file from the repository and generate a highly concise summary.
Format your output exactly as a JSON object with two keys:
"purpose": A one-sentence explanation of what this file does.
"learning_objective": A one-sentence explanation of what a developer should focus on learning when reading this file.

File Path: %s
Content:
%s
`
	PromptChatSystem = `You are a helpful expert software engineer. You are answering a question about the following file.
File Path: %s
Content:
%s
`
)

func GenerateContext(provider, model, content, path string) (*GenerateResponse, error) {
	prompt := fmt.Sprintf(PromptGenerate, path, content)
	
	respText, err := callLLM(provider, model, "system", prompt, nil)
	if err != nil {
		return nil, err
	}

	// Try to parse the response as JSON. Clean up markdown code blocks if necessary.
	cleaned := strings.TrimPrefix(respText, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var res GenerateResponse
	err = json.Unmarshal([]byte(cleaned), &res)
	if err != nil {
		// Fallback if the model didn't return perfect JSON
		return &GenerateResponse{
			Purpose:           "Failed to parse AI response as JSON.",
			LearningObjective: respText,
		}, nil
	}

	return &res, nil
}

func Chat(provider, model, content, path string, history []Message, prompt string) (string, error) {
	systemPrompt := fmt.Sprintf(PromptChatSystem, path, content)
	return callLLM(provider, model, systemPrompt, prompt, history)
}

func callLLM(provider, model, systemPrompt, prompt string, history []Message) (string, error) {
	switch provider {
	case "Gemini":
		if model == "" { model = "gemini-1.5-pro" }
		return callGemini(model, systemPrompt, prompt, history)
	case "OpenAI":
		if model == "" { model = "gpt-4o" }
		return callOpenAIFormat("https://api.openai.com/v1/chat/completions", os.Getenv("OPENAI_API_KEY"), model, systemPrompt, prompt, history)
	case "Anthropic":
		if model == "" { model = "claude-3-5-sonnet-20241022" }
		return callAnthropic(model, systemPrompt, prompt, history)
	case "Groq":
		if model == "" { model = "llama-3.3-70b-versatile" }
		return callOpenAIFormat("https://api.groq.com/openai/v1/chat/completions", os.Getenv("GROQ_API_KEY"), model, systemPrompt, prompt, history)
	case "Ollama (Local)":
		if model == "" { model = "llama3" }
		return callOpenAIFormat("http://localhost:11434/v1/chat/completions", "dummy", model, systemPrompt, prompt, history)
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

// --- OpenAI Format (Reused for OpenAI, Groq, Ollama) ---

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func callOpenAIFormat(url, apiKey, model, systemPrompt, prompt string, history []Message) (string, error) {
	if apiKey == "" && !strings.Contains(url, "localhost") {
		return "", errors.New("API key not found in .env")
	}

	messages := []openAIMessage{
		{Role: "system", Content: systemPrompt},
	}
	for _, h := range history {
		messages = append(messages, openAIMessage{Role: h.Role, Content: h.Content})
	}
	if prompt != "" && prompt != systemPrompt {
		messages = append(messages, openAIMessage{Role: "user", Content: prompt})
	}

	reqBody, _ := json.Marshal(openAIRequest{
		Model:    model,
		Messages: messages,
	})

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("no choices returned")
	}
	return result.Choices[0].Message.Content, nil
}

// --- Gemini ---

func callGemini(model, systemPrompt, prompt string, history []Message) (string, error) {
        apiKey := os.Getenv("GEMINI_API_KEY")
        if apiKey == "" {
                return "", errors.New("GEMINI_API_KEY not found in .env")
        }
        url := "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent?key=" + apiKey

	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Role  string `json:"role"`
		Parts []part `json:"parts"`
	}

	var contents []content
	// Gemini uses 'user' and 'model'
	
	// Add history
	for _, h := range history {
		role := h.Role
		if role == "assistant" {
			role = "model"
		}
		contents = append(contents, content{
			Role:  role,
			Parts: []part{{Text: h.Content}},
		})
	}
	
	// Add current prompt
	finalPrompt := prompt
	if systemPrompt != "" && prompt == systemPrompt {
	    // If it's a generation request, just send the prompt
	} else if systemPrompt != "" {
	    finalPrompt = "System Instructions:\n" + systemPrompt + "\n\nUser Question:\n" + prompt
	}
	
	contents = append(contents, content{
		Role:  "user",
		Parts: []part{{Text: finalPrompt}},
	})

	reqBody, _ := json.Marshal(map[string]interface{}{
		"contents": contents,
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Gemini API error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("empty response from Gemini")
	}
	return result.Candidates[0].Content.Parts[0].Text, nil
}

// --- Anthropic ---

func callAnthropic(model, systemPrompt, prompt string, history []Message) (string, error) {
        apiKey := os.Getenv("ANTHROPIC_API_KEY")
        if apiKey == "" {
                return "", errors.New("ANTHROPIC_API_KEY not found in .env")
        }
        url := "https://api.anthropic.com/v1/messages"

        type message struct {
                Role    string `json:"role"`
                Content string `json:"content"`
        }

        var messages []message
        for _, h := range history {
                messages = append(messages, message{Role: h.Role, Content: h.Content})
        }
        if prompt != "" && prompt != systemPrompt {
                messages = append(messages, message{Role: "user", Content: prompt})
        } else if prompt == systemPrompt {
            messages = append(messages, message{Role: "user", Content: prompt})
            systemPrompt = "" // Clear it so we don't send it twice
        }

        reqData := map[string]interface{}{
                "model":      model,
                "max_tokens": 1024,
                "messages":   messages,
        }
	if systemPrompt != "" {
		reqData["system"] = systemPrompt
	}

	reqBody, _ := json.Marshal(reqData)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Anthropic API error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", errors.New("empty response from Anthropic")
	}
	return result.Content[0].Text, nil
}