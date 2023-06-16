package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/patrickmn/go-cache"
	gogpt "github.com/sashabaranov/go-openai"
	"github.com/wonderivan/logger"
	"github.com/zhang19523zhao/wechat-all-gpt/config"
	"math/rand"
	"os"
	"strings"
	"time"
)

type ChatGpt struct {
	client *gogpt.Client
	ctx    context.Context
}

var Gpt ChatGpt

// 企业微信信息缓存
var conversationCache = cache.New(5*time.Minute, 5*time.Minute)

func NewGPT(apiKey string) *ChatGpt {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-ctx.Done()
		cancel()
	}()

	return &ChatGpt{
		client: gogpt.NewClient(apiKey),
		ctx:    ctx,
	}
}

func (*ChatGpt) AskGpt(question, conversationId, tp string, contentSize int) (string, error) {
	chat := NewGPT(config.LoadConfig().ApiKey)
	defer chat.Close()
	if tp == "image" {
		filep, err := chat.ChatImage(question)
		return filep, err
	}

	messages := []gogpt.ChatCompletionMessage{}

	key := fmt.Sprintf("cache:conversation:", conversationId)
	data, found := conversationCache.Get(key)
	if found {
		messages = data.([]gogpt.ChatCompletionMessage)
	}
	messages = append(messages, gogpt.ChatCompletionMessage{
		Role:    gogpt.ChatMessageRoleSystem,
		Content: question,
	})

	conversationLen := contentSize
	if conversationLen > len(messages) {
		conversationLen = len(messages)
	}
	messages = messages[len(messages)-conversationLen:]
	conversationCache.Set(key, messages, 12*time.Hour)

	answer, err := chat.Chat(messages)
	if err != nil {
		return "机器人太累了请稍后重试!", err
	}
	return answer, nil
}

func (c *ChatGpt) Chat(messages []gogpt.ChatCompletionMessage) (string, error) {
	msg := gogpt.ChatCompletionMessage{}
	msg.Role = gogpt.ChatMessageRoleSystem
	req := gogpt.ChatCompletionRequest{
		Model:    gogpt.GPT3Dot5Turbo,
		Messages: messages,
	}

	fmt.Printf("Messages:%v\n", messages)
	resp, err := c.client.CreateChatCompletion(c.ctx, req)
	if err != nil {
		logger.Error(fmt.Sprintf("CreateChatCompletion: %v\n", err))
		return "", errors.New(fmt.Sprintf("CreateChatCompletion: %v\n", err))
	}
	answer := resp.Choices[0].Message.Content
	logger.Info(fmt.Sprintf("Gpt: %s\n", answer))
	for len(answer) > 0 {
		if answer[0] == '\n' {
			answer = answer[1:]
		} else {
			break
		}
	}
	return answer, nil
}

func (c *ChatGpt) ChatImage(question string) (filePath string, err error) {
	prompt := strings.TrimPrefix(question, "/image")
	req := gogpt.ImageRequest{
		Prompt:         prompt,
		N:              1,
		Size:           gogpt.CreateImageSize1024x1024,
		ResponseFormat: gogpt.CreateImageResponseFormatB64JSON,
	}
	resp, err := c.client.CreateImage(c.ctx, req)
	if err != nil || len(resp.Data) == 0 {
		return "", errors.New(fmt.Sprintf("创建图片失败: %v\n", err))
	}
	imageBytes, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Image base64 decode error: %v\n", err))
	}
	date := time.Now().Format("2006-01-02")
	imageDir := fmt.Sprintf("images/%s", date)
	err = os.MkdirAll(imageDir, 0700)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Create image dirdctory err: %v\n", err))
	}
	imageFileName := fmt.Sprintf("%s.png", RandomString(16))
	filep := fmt.Sprintf("%s/%s", imageDir, imageFileName)
	err = os.WriteFile(filep, imageBytes, 0600)
	return filep, nil
}

func RandomString(n int) string {
	var letter []rune
	lowerChars := "abcdefghijklmnopqrstuvwxyz"
	numberChars := "0123456789"
	chars := fmt.Sprintf("%s%s", lowerChars, numberChars)
	letter = []rune(chars)
	var str string
	b := make([]rune, n)
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := range b {
		b[i] = letter[seededRand.Intn(len(letter))]
	}
	str = string(b)
	return str
}

func (c *ChatGpt) Close() {
	c.ctx.Done()
}
