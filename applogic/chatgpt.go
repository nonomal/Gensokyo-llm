package applogic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/hoshinonyaruko/gensokyo-llm/config"
	"github.com/hoshinonyaruko/gensokyo-llm/structs"
	"github.com/hoshinonyaruko/gensokyo-llm/utils"
)

// 用于存储每个conversationId的最后一条消息内容
var (
	// lastResponses 存储每个真实 conversationId 的最后响应文本
	lastResponses sync.Map
	// conversationMap 存储 msg.ConversationID 到真实 conversationId 的映射
	conversationMap       sync.Map
	lastCompleteResponses sync.Map // 存储每个conversationId的完整累积信息
)

func (app *App) ChatHandlerChatgpt(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg structs.Message
	err := json.NewDecoder(r.Body).Decode(&msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	msg.Role = "user"

	if msg.ConversationID == "" {
		msg.ConversationID = utils.GenerateUUID()
		app.createConversation(msg.ConversationID)
	}

	userMessageID, err := app.addMessage(msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 获取历史信息
	var history []structs.Message
	if msg.ParentMessageID != "" {
		history, err = app.getHistory(msg.ConversationID, msg.ParentMessageID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 截断历史信息
		history = truncateHistoryGpt(history, msg.Text)
	}

	// 获取系统提示词
	systemPromptContent := config.SystemPrompt()
	if systemPromptContent != "0" {
		systemPrompt := structs.Message{
			Text: systemPromptContent,
			Role: "system",
		}
		// 将系统提示词添加到历史信息的开始
		history = append([]structs.Message{systemPrompt}, history...)
	}

	// 构建请求到ChatGPT API
	model := config.GetGptModel()
	apiURL := config.GetGptApiPath()
	token := config.GetGptToken()

	// 构造消息历史和当前消息
	messages := []map[string]interface{}{}
	for _, hMsg := range history {
		messages = append(messages, map[string]interface{}{
			"role":    hMsg.Role,
			"content": hMsg.Text,
		})
	}
	messages = append(messages, map[string]interface{}{
		"role":    "user",
		"content": msg.Text,
	})

	//是否安全模式
	safemode := config.GetGptSafeMode()
	useSSe := config.GetuseSse()

	// 构建请求体
	requestBody := map[string]interface{}{
		"model":     model,
		"messages":  messages,
		"safe_mode": safemode,
		"stream":    useSSe,
	}
	fmt.Printf("chatgpt requestBody :%v", requestBody)
	requestBodyJSON, _ := json.Marshal(requestBody)

	// 准备HTTP请求
	client := &http.Client{}
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error sending request to ChatGPT API: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if !config.GetuseSse() {
		// 处理响应
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read response body: %v", err), http.StatusInternalServerError)
			return
		}
		// 假设已经成功发送请求并获得响应，responseBody是响应体的字节数据
		var apiResponse struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(responseBody, &apiResponse); err != nil {
			http.Error(w, fmt.Sprintf("Error unmarshaling API response: %v", err), http.StatusInternalServerError)
			return
		}

		// 从API响应中获取回复文本
		responseText := ""
		if len(apiResponse.Choices) > 0 {
			responseText = apiResponse.Choices[0].Message.Content
		}

		// 添加助理消息
		assistantMessageID, err := app.addMessage(structs.Message{
			ConversationID:  msg.ConversationID,
			ParentMessageID: userMessageID,
			Text:            responseText,
			Role:            "assistant",
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 构造响应数据，包括回复文本、对话ID、消息ID，以及使用情况（用例中未计算，可根据需要添加）
		responseMap := map[string]interface{}{
			"response":       responseText,
			"conversationId": msg.ConversationID,
			"messageId":      assistantMessageID,
			// 在此实际使用情况中，应该有逻辑来填充totalUsage
			// 此处仅为示例，根据实际情况来调整
			"details": map[string]interface{}{
				"usage": structs.UsageInfo{
					PromptTokens:     0, // 示例值，需要根据实际情况计算
					CompletionTokens: 0, // 示例值，需要根据实际情况计算
				},
			},
		}

		// 设置响应头部为JSON格式
		w.Header().Set("Content-Type", "application/json")
		// 将响应数据编码为JSON并发送
		if err := json.NewEncoder(w).Encode(responseMap); err != nil {
			http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// 设置SSE相关的响应头部
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		reader := bufio.NewReader(resp.Body)
		var responseTextBuilder strings.Builder
		var totalUsage structs.GPTUsageInfo
		if config.GetGptSseType() == 1 {
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break // 流结束
					}
					// 处理错误
					fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf("读取流数据时发生错误: %v", err))
					flusher.Flush()
					continue
				}

				if strings.HasPrefix(line, "data: ") {
					eventDataJSON := line[5:] // 去掉"data: "前缀

					// 解析JSON数据
					var eventData structs.GPTEventData
					if err := json.Unmarshal([]byte(eventDataJSON), &eventData); err != nil {
						fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf("解析事件数据出错: %v", err))
						flusher.Flush()
						continue
					}

					// 遍历choices数组，累积所有文本内容
					for _, choice := range eventData.Choices {
						responseTextBuilder.WriteString(choice.Delta.Content)
					}

					// 如果存在需要发送的临时响应数据（例如，在事件流中间点）
					// 注意：这里暂时省略了使用信息的处理，因为示例输出中没有包含这部分数据
					tempResponseMap := map[string]interface{}{
						"response":       responseTextBuilder.String(),
						"conversationId": msg.ConversationID, // 确保msg.ConversationID已经定义并初始化
						// "details" 字段留待进一步处理，如有必要
					}
					tempResponseJSON, _ := json.Marshal(tempResponseMap)
					fmt.Fprintf(w, "data: %s\n\n", string(tempResponseJSON))
					flusher.Flush()
				}
			}
		} else {
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break // 流结束
					}
					fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf("读取流数据时发生错误: %v", err))
					flusher.Flush()
					continue
				}

				if strings.HasPrefix(line, "data: ") {
					eventDataJSON := line[5:] // 去掉"data: "前缀
					if eventDataJSON[1] != '{' {
						fmt.Println("非JSON数据,跳过:", eventDataJSON)
						continue
					}

					var eventData structs.GPTEventData
					if err := json.Unmarshal([]byte(eventDataJSON), &eventData); err != nil {
						fmt.Fprintf(w, "data: %s\n\n", fmt.Sprintf("解析事件数据出错: %v", err))
						flusher.Flush()
						continue
					}

					conversationId := eventData.ID // 假设conversationId从事件数据的ID字段获取
					conversationMap.Store(msg.ConversationID, conversationId)
					//读取完整信息
					completeResponse, _ := lastCompleteResponses.LoadOrStore(conversationId, "")

					// 检索上一次的响应文本
					lastResponse, _ := lastResponses.LoadOrStore(conversationId, "")
					lastResponseText := lastResponse.(string)

					newContent := ""
					for _, choice := range eventData.Choices {
						if strings.HasPrefix(choice.Delta.Content, lastResponseText) {
							// 如果新内容以旧内容开头，剔除旧内容部分，只保留新增的部分
							newContent += choice.Delta.Content[len(lastResponseText):]
						} else {
							// 如果新内容不以旧内容开头，可能是并发情况下的新消息，直接使用新内容
							newContent += choice.Delta.Content
						}
					}

					// 更新存储的完整累积信息
					updatedCompleteResponse := completeResponse.(string) + newContent
					lastCompleteResponses.Store(conversationId, updatedCompleteResponse)

					// 使用累加的新内容更新存储的最后响应状态
					if newContent != "" {
						lastResponses.Store(conversationId, newContent)
					}

					// 发送新增的内容
					if newContent != "" {
						tempResponseMap := map[string]interface{}{
							"response":       newContent,
							"conversationId": conversationId,
						}
						tempResponseJSON, _ := json.Marshal(tempResponseMap)
						fmt.Fprintf(w, "data: %s\n\n", string(tempResponseJSON))
						flusher.Flush()
					}
				}
			}
		}

		// 在所有事件处理完毕后发送最终响应
		responseText := responseTextBuilder.String()
		assistantMessageID, err := app.addMessage(structs.Message{
			ConversationID:  msg.ConversationID,
			ParentMessageID: userMessageID,
			Text:            responseText,
			Role:            "assistant",
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 在所有事件处理完毕后发送最终响应
		// 首先从 conversationMap 获取真实的 conversationId
		if actualConversationId, ok := conversationMap.Load(msg.ConversationID); ok {
			if finalContent, ok := lastCompleteResponses.Load(actualConversationId); ok {
				finalResponseMap := map[string]interface{}{
					"response":       finalContent,
					"conversationId": actualConversationId,
					"messageId":      assistantMessageID,
					"details": map[string]interface{}{
						"usage": totalUsage,
					},
				}
				finalResponseJSON, _ := json.Marshal(finalResponseMap)
				fmt.Fprintf(w, "data: %s\n\n", string(finalResponseJSON))
				flusher.Flush()
			}
		}

	}
}

func truncateHistoryGpt(history []structs.Message, prompt string) []structs.Message {
	MAX_TOKENS := config.GetMaxTokenGpt()

	tokenCount := len(prompt)
	for _, msg := range history {
		tokenCount += len(msg.Text)
	}

	if tokenCount <= MAX_TOKENS {
		return history
	}

	// 第一步：移除所有助手回复
	truncatedHistory := []structs.Message{}
	for _, msg := range history {
		if msg.Role == "user" {
			truncatedHistory = append(truncatedHistory, msg)
		}
	}

	tokenCount = len(prompt)
	for _, msg := range truncatedHistory {
		tokenCount += len(msg.Text)
	}

	if tokenCount <= MAX_TOKENS {
		return truncatedHistory
	}

	// 第二步：从开始逐个移除消息，直到满足令牌数量限制
	for tokenCount > MAX_TOKENS && len(truncatedHistory) > 0 {
		tokenCount -= len(truncatedHistory[0].Text)
		truncatedHistory = truncatedHistory[1:]
	}

	return truncatedHistory
}