package controller

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/wonderivan/logger"
	"github.com/zhang19523zhao/wechat-all-gpt/common"
	"github.com/zhang19523zhao/wechat-all-gpt/config"
	"github.com/zhang19523zhao/wechat-all-gpt/model"
	"github.com/zhang19523zhao/wechat-all-gpt/service"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 企业微信 token 缓存，请求频次过高可能有一些额外的问题
var tokenCache = cache.New(5*time.Minute, 5*time.Minute)

// 微信公众号消息缓存
var wechatMpCache = cache.New(5*time.Minute, 5*time.Minute)

// 微信公众号用户每日消息限制
var wechatMpLimitCache = cache.New(24*time.Hour, 30*time.Second)

type AccessToken struct {
	Errcode     int    `json:"errcode"`
	Errmsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

var answerMp = ""
var timeSize5 = 5 * time.Second
var timeSize4 = 4 * time.Second
var timeSize = timeSize5

func TailkWeixin(c *gin.Context) {

	conf := config.LoadConfig().WechatApp.WechatUrl
	token := conf.Token
	receiverId := conf.Corpid
	encodingAeskey := conf.EncodingAeskey

	switch config.LoadConfig().Type {
	case "wechat_app":
		Vmsg := c.Query("msg_signature")
		Vtime := c.Query("timestamp")
		Vnonce := c.Query("nonce")

		crypt := common.NewWXBizMsgCrypt(token, encodingAeskey, receiverId, 1)
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		data, _ := crypt.DecryptMsg(Vmsg, Vtime, Vnonce, bodyBytes)
		var weixinUserAskMsg model.WeixinUserAskMsg
		if err := xml.Unmarshal(data, &weixinUserAskMsg); err != nil {
			logger.Error(fmt.Sprintf("xml 反序列化失败: %v\n", err))
			c.JSON(500, fmt.Sprintf("xml 反序列化失败: %v\n", err))
		}
		msgType := "text"
		question := weixinUserAskMsg.Content

		if strings.HasPrefix(question, "/image") {
			msgType = "image"
		}
		logger.Info(fmt.Sprintf("用户%s: %s\n", weixinUserAskMsg.FromUserName, question))
		go handleMsgRet(question, weixinUserAskMsg, conf.Corpid, conf.Corpsecret, msgType, config.LoadConfig().WeworkConversationSize)
	case "wechat_mp":

		fmt.Println("-----------------------------")
		var mpUserAskMsg model.WeixinMapAskMsg
		if err := c.ShouldBindXML(&mpUserAskMsg); err != nil {
			logger.Error(fmt.Sprintf("ShouldBindXML error: %v\n", err))
			return
		}

		// 判断用户是否超过了每日会话限制,如果消息额度为0则返回已经超出每日limit提问限制
		limit, found := wechatMpLimitCache.Get(mpUserAskMsg.FromUserName)

		if limit == 0 && found {
			fmt.Println("返回给用户次数已经用完了")
			respText := fmt.Sprintf("您的次数已经用完,公众号每日限制:%d次\n\n <a href='http://ai.zhgolang.com/'>使用AI高级版，立即提问>></a>", config.LoadConfig().WwechatMp.Limit)
			resp := NewReplyMp(mpUserAskMsg, respText)
			//fmt.Println("回复用户消息", answer.(string))
			c.XML(200, resp)
		}

		// 如果没有找到则是第一次提问则给他初始化
		if !found {
			lt := config.LoadConfig().WwechatMp.Limit
			fmt.Println("lt----:", lt)
			wechatMpLimitCache.Set(mpUserAskMsg.FromUserName, lt, 24*time.Hour)
		}

		done := make(chan bool, 1)
		timeout := 5000 * time.Millisecond
		ans := "【机器人处理中回复任意数字查看答案】"
		ansLong := "【未完待续，回复任意文字以继续】"
		// 用户消息
		//key := fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId)
		// 通过用户查看缓存中是否有答案
		answer, found := wechatMpCache.Get(mpUserAskMsg.FromUserName)
		// 查看是否有缓存超过长度的消息
		answerLong, foundLong := wechatMpCache.Get(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName))
		if found && !foundLong {
			resp := NewReplyMp(mpUserAskMsg, answer.(string))
			fmt.Println("回复用户消息", answer.(string))
			c.XML(200, resp)
			if answer != ans {
				wechatMpCache.Delete(mpUserAskMsg.FromUserName)
			}
			return
		}

		if foundLong {
			answers := answerLong.([]string)
			answer := answers[0]
			resp := NewReplyMp(mpUserAskMsg, answer)
			c.XML(200, resp)
			if len(answers) == 1 {
				wechatMpCache.Delete(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName))
				wechatMpCache.Delete(mpUserAskMsg.FromUserName)
			} else {
				answers = answers[1:]
				wechatMpCache.Set(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName), answers, 2*time.Minute)
			}
			return
		}

		// 查看此问题是否为第一次询问
		questionCount, _ := wechatMpCache.Get(mpUserAskMsg.MsgId)
		if questionCount == 2 {
			timeout = 4900 * time.Millisecond
		}
		if questionCount == nil {
			var err error
			var answer string
			questionCount = 1
			wechatMpCache.Set(mpUserAskMsg.MsgId, questionCount, 2*time.Minute)
			go func() {
				// 测试的时候关闭
				answer, err = service.Gpt.AskGpt(mpUserAskMsg.Content, mpUserAskMsg.FromUserName, "text", config.LoadConfig().WeworkConversationSize)
				lt, _ := wechatMpLimitCache.Get(mpUserAskMsg.FromUserName)
				numLimit := lt.(int)
				numLimit -= 1
				wechatMpLimitCache.Set(mpUserAskMsg.FromUserName, numLimit, 24*time.Hour)
				// 测试
				//answer = "客户端-go是Kubernetes官方提供的一种用于访问Kubernetes API的库。借助客户端-go，我们可以轻松地与Kubernetes API进行交互，例如创建、更新、删除Pod、Service等资源。下面是一个简单的示例，演示如何使用客户端-go来获取Kubernetes集群中的所有Pod资源。\n\n首先，我们需要导入客户端-go的库：\n\n```go\nimport (\n    \"k8s.io/client-go/kubernetes\"\n    \"k8s.io/client-go/rest\"\n    \"k8s.io/client-go/tools/clientcmd\"\n)\n```\n\n然后，我们需要获取一个可以访问Kubernetes API的客户端。有两种方式可以获取到这个客户端，一种是使用Kubernetes集群自带的服务账号（ServiceAccount），另一种是使用Kubernetes集群中已有的kubeconfig文件。这里我们用第二种方式获取客户端：\n\n```go\nfunc getClient() (*kubernetes.Clientset, error) {\n    var config *rest.Config\n    if home := homeDir(); home != \"\" {\n        kubeconfig := filepath.Join(home, \".kube\", \"config\")\n        config, err = clientcmd.BuildConfigFromFlags(\"\", kubeconfig)\n        if err != nil {\n            return nil, err\n        }\n    } else {\n        config, err = rest.InClusterConfig()\n        if err != nil {\n            return nil, err\n        }\n    }\n    clientset, err := kubernetes.NewForConfig(config)\n    if err != nil {\n        return nil, err\n    }\n    return clientset, nil\n}\n```\n\n上述代码中，getClient()函数返回一个客户端-go的Clientset对象。getClient()函数和homeDir()函数实现如下：\n\n```go\n// 获取当前用户的家目录\nfunc homeDir() string {\n    if h := os.Getenv(\"HOME\"); h != \"\" {\n        return h\n    }\n    return os.Getenv(\"USERPROFILE\") // Windows\n}\n\n// 创建Kubernetes客户端\nclientset, err := getClient()\nif err != nil {\n    log.Errorf(\"Error creating clientset: %s\", err)\n    return\n}\n```\n\n在创建Kubernetes客户端之后，我们就可以使用它来获取Pod资源了：\n\n```go\npods, err := clientset.CoreV1().Pods(\"\").List(context.Background(), metav1.ListOptions{})\nif err != nil {\n    log.Errorf(\"Error listing pods: %s\", err)\n    return\n}\nfor _, pod := range pods.Items {\n    fmt.Printf(\"Found pod %s in namespace %s\\n\", pod.Name, pod.Namespace)\n}\n```\n\n上述代码中，我们使用clientset.CoreV1().Pods(\"\")来获取所有Pod资源，然后遍历所有的Pod，并输出它们的名称和命名空间。注意，在这个简单的示例中我们没有进行错误处理，但在实际的应用中一定要考虑到错误场景。"
				// 如果消息长度超过了650则分隔一下，用ansLong分隔
				ranswer := []rune(answer)
				fmt.Println("消息的长度为:", len(ranswer))
				answers := []string{}

				num := len(ranswer) / 650
				fmt.Println("分段次数@:", num)
				if num >= 1 {

					for i := 1; i <= num; i++ {
						tmpStr := string(ranswer[650*(i-1) : 650*i])
						tmpAns := tmpStr + ansLong
						answers = append(answers, tmpAns)
					}
					answers = append(answers, string(ranswer[num*650:]))
					wechatMpCache.Set(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName), answers, 2*time.Minute)
				} else {
					if err != nil {
						logger.Error(fmt.Sprintf("获取gpt回复失败: %v\n", err))
						wechatMpCache.Set(mpUserAskMsg.FromUserName, "服务器火爆请稍后重试", 2*time.Minute)
						c.XML(200, NewReplyMp(mpUserAskMsg, fmt.Sprintf("获取gpt回复失败: %v\n", err)))
						return
					}
				}
				// 获取到答案后设置缓存
				wechatMpCache.Set(mpUserAskMsg.FromUserName, answer, 2*time.Minute)

				done <- true
			}()

			select {
			case <-done:
				// 查看是否有缓存超过长度的消息
				answerLong, found := wechatMpCache.Get(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName))
				if found {
					answers := answerLong.([]string)
					answer := answers[0]
					resp := NewReplyMp(mpUserAskMsg, answer)
					c.XML(200, resp)
					if len(answers) == 1 {
						wechatMpCache.Delete(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName))
					} else {
						answers = answers[1:]
					}
					wechatMpCache.Set(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName), answers, 2*time.Minute)
					return
				} else {
					resp := NewReplyMp(mpUserAskMsg, answer)
					c.XML(200, resp)
					wechatMpCache.Delete(mpUserAskMsg.FromUserName)
					return
				}

			case <-time.After(timeout):
				logger.Warn(fmt.Sprintf("第%d次请求超时问题: %s\n", questionCount, mpUserAskMsg.Content))
			}

		} else {
			// 处理不是第一次询问
			select {
			case <-done:
				// 查看是否有缓存超过长度的消息
				answerLong, found := wechatMpCache.Get(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName))
				if found {
					answers := answerLong.([]string)
					answer := answers[0]
					resp := NewReplyMp(mpUserAskMsg, answer)
					c.XML(200, resp)
					if len(answers) == 1 {
						wechatMpCache.Delete(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName))
					} else {
						answers = answers[1:]
					}
					wechatMpCache.Set(fmt.Sprintf("ansLong%s", mpUserAskMsg.FromUserName), answers, 2*time.Minute)
					return
				} else {
					answer, found := wechatMpCache.Get(mpUserAskMsg.FromUserName)
					if found {
						resp := NewReplyMp(mpUserAskMsg, answer.(string))
						c.XML(200, resp)
						wechatMpCache.Delete(mpUserAskMsg.FromUserName)
						return
					}
				}
			case <-time.After(timeout):
				count := questionCount.(int)
				count++
				logger.Warn(fmt.Sprintf("第%d次请求超时问题: %s\n", count, mpUserAskMsg.Content))
				// 设置当前请求的次数
				wechatMpCache.Set(mpUserAskMsg.MsgId, count, 2*time.Minute)
				answer, found := wechatMpCache.Get(mpUserAskMsg.FromUserName)
				if count == 3 {
					resp := NewReplyMp(mpUserAskMsg, ans)
					c.XML(200, resp)
					if found {
						ans = answer.(string)
					}
					wechatMpCache.Set(mpUserAskMsg.FromUserName, ans, 2*time.Minute)
					return
				}
			}
		}
		return
	}
}

func NewReplyMp(mpUserAskMsg model.WeixinMapAskMsg, answer string) model.ReplyMp {
	return model.ReplyMp{
		ToUserName:   mpUserAskMsg.FromUserName,
		FromUserName: mpUserAskMsg.ToUserName,
		CreateTime:   1,
		MsgType:      "text",
		Content:      answer,
	}
}

func handleMsgRet(question string, weixinUserAskMsg model.WeixinUserAskMsg, corpid, corpsecret, msgType string, contentSize int) {
	answer, err := service.Gpt.AskGpt(question, weixinUserAskMsg.FromUserName, msgType, contentSize)
	accessToken, err := accessToken(corpid, corpsecret)
	if err != nil {
		logger.Error(err)
	}
	if err != nil {
		answer = "服务器火爆请稍后重试"
	}
	resp := model.ReplyAll{}
	if msgType == "image" {
		mid, err := uplaodTmp(accessToken, msgType, answer)
		if err != nil {
			logger.Error(err)
		}
		resp.RepImage = model.ReplyImage{
			Touser:  weixinUserAskMsg.FromUserName,
			Msgtype: "image",
			Agentid: weixinUserAskMsg.AgentID,
			Image: struct {
				MediaId string `json:"media_id,omitempty"`
			}{mid},
		}
	} else if msgType == "text" {
		resp.RepText = model.ReplyMsg{
			Touser:  weixinUserAskMsg.FromUserName,
			Msgtype: "text",
			Agentid: weixinUserAskMsg.AgentID,
			Text: struct {
				Content string `json:"content,omitempty"`
			}{Content: answer},
		}
	}

	if err := callTalk(resp, msgType, accessToken); err != nil {
		logger.Error(err)
		return
	}
}

func accessToken(corpid, corpsecret string) (string, error) {
	tokenCacheKey := "tokenCache"
	data, found := tokenCache.Get(tokenCacheKey)
	if found {
		return fmt.Sprintf("%v", data), nil
	}

	urlBase := "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s"
	url := fmt.Sprintf(urlBase, corpid, corpsecret)
	method := "GET"
	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		logger.Error(fmt.Sprintf("accessToken new request 失败: %v\n", err))
		return "", errors.New(fmt.Sprintf("accessToken new request 失败: %v\n", err))
	}
	res, err := client.Do(req)
	if err != nil {
		logger.Error(fmt.Sprintf("获取token失败: %v\n", err))
		return "", errors.New(fmt.Sprintf("获取token失败: %v\n", err))
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Error(fmt.Sprintf("获取token Body失败: %v\n", err))
		return "", errors.New(fmt.Sprintf("获取token Body失败: %v\n", err))
	}

	var accessToken AccessToken
	if err := json.Unmarshal(body, &accessToken); err != nil {
		logger.Error("Get token err: %v\n", err)
	}
	token := accessToken.AccessToken

	tokenCache.Set(tokenCacheKey, token, 5*time.Minute)
	return token, nil
}

func callTalk(reply model.ReplyAll, msgType, accessToken string) error {
	url := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + accessToken
	method := "POST"
	var data []byte
	var err error

	if msgType == "image" {
		rep := reply.RepImage
		data, err = json.Marshal(rep)
	} else if msgType == "text" {
		rep := reply.RepText
		data, err = json.Marshal(rep)
	}

	if err != nil {
		return errors.New(fmt.Sprintf("序列化reply失败: %v\n", err))
	}
	reBody := string(data)
	payload := strings.NewReader(reBody)
	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")
	res, err := client.Do(req)
	defer res.Body.Close()
	if err != nil {
		return errors.New(fmt.Sprintf("回复消息失败: %v\n", err))
	}
	body, err := io.ReadAll(res.Body)
	//logger.Info("Reply body: %s\n", string(body))
	fmt.Println(string(body))
	return nil
}

func uplaodTmp(token, msgType, filePath string) (string, error) {
	url := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/media/upload?access_token=%s&type=%s", token, msgType)
	method := "POST"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	file, err := os.Open(filePath)
	fmt.Println("filepath:", filePath)
	if err != nil {
		return "", errors.New(fmt.Sprintf("打开文件失败: %v\n", err))
	}
	defer file.Close()
	part1, err := writer.CreateFormFile("filename", filepath.Base(filePath))
	if err != nil {
		return "", errors.New(fmt.Sprintf("CreateFormFile err: %v\n", err))
	}

	_, err = io.Copy(part1, file)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Copy faild: %v\n", err))
	}
	err = writer.Close()
	if err != nil {
		return "", errors.New(fmt.Sprintf("writer close err: %v\n", err))
	}
	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)
	if err != nil {
		return "", errors.New(fmt.Sprintf("NewReauest err %v\n", err))
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", errors.New(fmt.Sprintf("ReadAll body err: %v\n", err))
	}
	rp := new(model.UploadTmpResp)
	json.Unmarshal(body, rp)
	return rp.MediaId, nil
}
