package config

import (
	"encoding/json"
	"github.com/zhang19523zhao/wechat-all-gpt/model"
	"log"
	"os"
	"sync"
)

// Configuration 项目配置
type Configuration struct {
	// gpt apikey
	ApiKey string `json:"api_key"`
	// 上下文对话能力，可以根据需要修改对话长度
	WeworkConversationSize int `json:"wework_conversation_size"`
	// 端口
	Port string `json:"port"`
	// 服务类型
	Type string `json:"type"`
	// 企业微信应用
	WechatApp model.WechatApp `json:"wechat_app"`
	// 微信公众号
	WwechatMp model.WechatMp `json:"wechat_mp"`
}

var config *Configuration
var once sync.Once

// LoadConfig加载配置
func LoadConfig() *Configuration {
	once.Do(func() {
		config = &Configuration{}
		// 从文件中读取
		f, err := os.Open("config.json")
		if err != nil {
			log.Fatalf("打开配置文件失败: %v\n", err)
			return
		}
		defer f.Close()

		decoder := json.NewDecoder(f)
		if err := decoder.Decode(config); err != nil {
			log.Fatalf("Decode config 失败: %v\n", err)
			return
		}
	})
	return config
}
