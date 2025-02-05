package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type OpenAIRequest struct {
	Messages []Message `json:"messages"`
	Model    string    `json:"model"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CreateConversationRequest struct {
	DeploymentId          string `json:"deploymentId"`
	Name                  string `json:"name"`
	ExternalApplicationId string `json:"externalApplicationId"`
}

type CreateConversationResponse struct {
	Success bool `json:"success"`
	Result  struct {
		DeploymentConversationId string `json:"deploymentConversationId"`
		ExternalApplicationId    string `json:"externalApplicationId"`
	} `json:"result"`
}

type ChatRequest struct {
	RequestId                string     `json:"requestId"`
	DeploymentConversationId string     `json:"deploymentConversationId"`
	Message                  string     `json:"message"`
	IsDesktop                bool       `json:"isDesktop"`
	ChatConfig               ChatConfig `json:"chatConfig"`
	LlmName                  string     `json:"llmName"`
	ExternalApplicationId    string     `json:"externalApplicationId"`
}

type ChatConfig struct {
	Timezone string `json:"timezone"`
	Language string `json:"language"`
}

type OpenAIStreamResponse struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

type OpenAIResponse struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type AbacusResponse struct {
	Type              string  `json:"type"`
	Temp              bool    `json:"temp"`
	IsSpinny          bool    `json:"isSpinny"`
	Segment           string  `json:"segment"`
	Title             string  `json:"title"`
	IsGeneratingImage bool    `json:"isGeneratingImage"`
	MessageId         string  `json:"messageId"`
	Counter           int     `json:"counter"`
	Message_id        string  `json:"message_id"`
	Token             *string `json:"token,omitempty"`
	End               bool    `json:"end,omitempty"`
	Success           bool    `json:"success,omitempty"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/chat/completions" {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]string{
			"status":  "Abacus2Api Service Running...",
			"message": "MoLoveSze...",
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 请求", http.StatusMethodNotAllowed)
		return
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "未提供有效的 Authorization header", http.StatusUnauthorized)
		return
	}
	cookie := strings.TrimPrefix(authHeader, "Bearer ")

	var requestBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, fmt.Sprintf("请求格式错误: %v", err), http.StatusBadRequest)
		return
	}

	isStream := requestBody.Stream

	convResp, err := createConversation(cookie)
	if err != nil {
		http.Error(w, "创建会话失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	message := ""
	var systemPrompt string
	var contextMessages []Message

	if len(requestBody.Messages) > 0 {
		message = requestBody.Messages[len(requestBody.Messages)-1].Content
		for _, msg := range requestBody.Messages[:len(requestBody.Messages)-1] {
			if msg.Role == "system" {
				systemPrompt = msg.Content
			} else {
				contextMessages = append(contextMessages, Message{Role: msg.Role, Content: msg.Content})
			}
		}
	}

	fullMessage := message
	if systemPrompt != "" {
		fullMessage = fmt.Sprintf("System: %s\n\n%s", systemPrompt, message)
	}
	if len(contextMessages) > 0 {
		contextStr := ""
		for _, ctx := range contextMessages {
			contextStr += fmt.Sprintf("%s: %s\n", ctx.Role, ctx.Content)
		}
		fullMessage = fmt.Sprintf("Previous conversation:\n%s\nCurrent message: %s", contextStr, message)
	}

	chatReq := ChatRequest{
		RequestId:                uuid.New().String(),
		DeploymentConversationId: convResp.Result.DeploymentConversationId,
		Message:                  fullMessage,
		IsDesktop:                true,
		ChatConfig: ChatConfig{
			Timezone: "Asia/Hong_Kong",
			Language: "zh-CN",
		},
		LlmName:               requestBody.Model,
		ExternalApplicationId: convResp.Result.ExternalApplicationId,
	}

	if isStream {
		err = sendStreamResponse(w, cookie, chatReq)
	} else {
		err = sendNonStreamResponse(w, cookie, chatReq)
	}

	if err != nil {
		http.Error(w, "发送消息失败", http.StatusInternalServerError)
		return
	}
}

func createConversation(cookie string) (*CreateConversationResponse, error) {
	reqBody := CreateConversationRequest{
		DeploymentId:          "d892fb336",
		Name:                  "New Chat",
		ExternalApplicationId: "ca852b1e2",
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://pa002.abacus.ai/cluster-proxy/api/createDeploymentConversation", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	setHeaders(req, cookie)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result CreateConversationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func sendStreamResponse(w http.ResponseWriter, cookie string, chatReq ChatRequest) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	jsonData, err := json.Marshal(chatReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://pa002.abacus.ai/api/_chatLLMSendMessageSSE", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	setHeaders(req, cookie)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Fprintf(w, "data: [DONE]\n\n")
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var abacusResp AbacusResponse
		if err := json.Unmarshal([]byte(line), &abacusResp); err != nil {
			continue
		}

		if abacusResp.Type == "text" && abacusResp.Title != "Thinking..." {
			streamResp := OpenAIStreamResponse{
				Id:      uuid.New().String(),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   chatReq.LlmName,
				Choices: []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					Index        int    `json:"index"`
					FinishReason string `json:"finish_reason,omitempty"`
				}{
					{
						Delta: struct {
							Content string `json:"content"`
						}{
							Content: abacusResp.Segment,
						},
						Index: 0,
					},
				},
			}

			jsonResp, err := json.Marshal(streamResp)
			if err != nil {
				return err
			}

			fmt.Fprintf(w, "data: %s\n\n", jsonResp)
		}

		if abacusResp.End {
			endResp := OpenAIStreamResponse{
				Id:      uuid.New().String(),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   chatReq.LlmName,
				Choices: []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					Index        int    `json:"index"`
					FinishReason string `json:"finish_reason,omitempty"`
				}{
					{
						Delta: struct {
							Content string `json:"content"`
						}{},
						Index:        0,
						FinishReason: "stop",
					},
				},
			}
			jsonResp, _ := json.Marshal(endResp)
			fmt.Fprintf(w, "data: %s\n\n", jsonResp)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			return nil
		}
	}

	return nil
}

func sendNonStreamResponse(w http.ResponseWriter, cookie string, chatReq ChatRequest) error {
	w.Header().Set("Content-Type", "application/json")

	jsonData, err := json.Marshal(chatReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "https://pa002.abacus.ai/api/_chatLLMSendMessageSSE", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	setHeaders(req, cookie)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var content strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var abacusResp AbacusResponse
		if err := json.Unmarshal([]byte(line), &abacusResp); err != nil {
			continue
		}

		if abacusResp.Type == "text" && abacusResp.Title != "Thinking..." {
			content.WriteString(abacusResp.Segment)
		}

		if abacusResp.End {
			break
		}
	}

	openAIResp := OpenAIResponse{
		Id:      uuid.New().String(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   chatReq.LlmName,
		Choices: []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					Role:    "assistant",
					Content: content.String(),
				},
				FinishReason: "stop",
			},
		},
	}

	return json.NewEncoder(w).Encode(openAIResp)
}

func setHeaders(req *http.Request, cookie string) {
	req.Header.Set("sec-ch-ua-platform", "Windows")
	req.Header.Set("sec-ch-ua", "\"Not(A:Brand\";v=\"99\", \"Microsoft Edge\";v=\"133\", \"Chromium\";v=\"133\"")
	req.Header.Set("sec-ch-ua-mobile", "?0")
	req.Header.Set("X-Abacus-Org-Host", "apps")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36 Edg/133.0.0.0")
	req.Header.Set("Sec-Fetch-Site", "same-site")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("host", "pa002.abacus.ai")
	req.Header.Set("Cookie", cookie)
}
