package main

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
		var mpUserAskMsg model.WeixinMapAskMsg
		if err := c.ShouldBindXML(&mpUserAskMsg); err != nil {
			logger.Error(fmt.Sprintf("ShouldBindXML error: %v\n", err))
			return
		}
		done := make(chan bool, 1)
		if _, found := wechatMpCache.Get(fmt.Sprintf("%sanswer1", mpUserAskMsg.FromUserName)); found {
			answer := ""
			ans2, _ := wechatMpCache.Get(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName))
			if ans2 != nil {
				answer = ans2.(string)
				wechatMpCache.Delete(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName))
				wechatMpCache.Delete(fmt.Sprintf("%sanswer1", mpUserAskMsg.FromUserName))
				wechatMpCache.Delete(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId))
			} else {
				answer = "机器人处理中，回复任意数字查看"
			}
			rp := model.ReplyMp{
				ToUserName:   mpUserAskMsg.FromUserName,
				FromUserName: mpUserAskMsg.ToUserName,
				CreateTime:   1,
				MsgType:      "text",
				Content:      answer,
			}
			c.XML(200, rp)
			return
		}
		question := mpUserAskMsg.Content
		logger.Info(fmt.Sprintf("用户%s问题: %s", mpUserAskMsg.FromUserName, question))

		data, found := wechatMpCache.Get(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId))
		if data == nil {
			data = 1
		}
		count := data.(int)
		if !found {
			go func() {
				answerMp, _ = service.Gpt.AskGpt(question, mpUserAskMsg.FromUserName, "text", config.LoadConfig().WeworkConversationSize)
				count, _ := wechatMpCache.Get(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId))
				fmt.Println("get cache count:", count)
				if count != 1 {
					wechatMpCache.Set(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName), answerMp, 2*time.Minute)
				}
				done <- true
			}()
			select {
			case <-done:
				rp := model.ReplyMp{
					ToUserName:   mpUserAskMsg.FromUserName,
					FromUserName: mpUserAskMsg.ToUserName,
					CreateTime:   1,
					MsgType:      "text",
					Content:      answerMp,
				}
				fmt.Printf("count:%s answerMp%s\n", count, answerMp)
				c.XML(200, rp)
				wechatMpCache.Delete(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName))
				wechatMpCache.Delete(fmt.Sprintf("%sanswer1", mpUserAskMsg.FromUserName))
				wechatMpCache.Delete(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId))
				return

			case <-time.After(5 * time.Second):
				wechatMpCache.Set(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId), count, time.Minute*2)
				return
			}
		}
		count++
		wechatMpCache.Set(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId), count, time.Minute*2)

		if count == 3 {
			timeSize = 4900 * time.Millisecond
		}

		select {
		case <-done:
			answer, _ := wechatMpCache.Get(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName))

			rp := model.ReplyMp{
				ToUserName:   mpUserAskMsg.FromUserName,
				FromUserName: mpUserAskMsg.ToUserName,
				CreateTime:   1,
				MsgType:      "text",
				Content:      answer.(string),
			}
			c.XML(200, rp)
			wechatMpCache.Delete(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName))
			wechatMpCache.Delete(fmt.Sprintf("%sanswer1", mpUserAskMsg.FromUserName))
			wechatMpCache.Delete(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId))
			return
		case <-time.After(timeSize):
			wechatMpCache.Set(mpUserAskMsg.FromUserName, count, time.Minute*2)
			answer, _ := wechatMpCache.Get(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName))
			if answer != nil {
				rp := model.ReplyMp{
					ToUserName:   mpUserAskMsg.FromUserName,
					FromUserName: mpUserAskMsg.ToUserName,
					CreateTime:   1,
					MsgType:      "text",
					Content:      answer.(string),
				}
				c.XML(200, rp)
				wechatMpCache.Delete(fmt.Sprintf("%sanswer2", mpUserAskMsg.FromUserName))
				wechatMpCache.Delete(fmt.Sprintf("%sanswer1", mpUserAskMsg.FromUserName))
				wechatMpCache.Delete(fmt.Sprintf("%s%s", mpUserAskMsg.FromUserName, mpUserAskMsg.MsgId))
				return
			}
			if ct, _ := wechatMpCache.Get(mpUserAskMsg.FromUserName); ct == 3 && answer == nil {
				rp := model.ReplyMp{
					ToUserName:   mpUserAskMsg.FromUserName,
					FromUserName: mpUserAskMsg.ToUserName,
					CreateTime:   1,
					MsgType:      "text",
					Content:      "机器人处理中，回复任意数字查看",
				}
				wechatMpCache.Set(fmt.Sprintf("%sanswer1", mpUserAskMsg.FromUserName), "机器人处理中，回复任意数字查看", 2*time.Minute)
				timeSize = timeSize5
				c.XML(200, rp)
				return
			}
		}
		return
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

//func handleMsgMpRet(question string, weixinMapAskMsg model.WeixinMapAskMsg, msgType string, contentSize int) {
//	answer, _ := service.Gpt.AskGpt(question, weixinMapAskMsg.FromUserName, msgType, contentSize)
//
//}

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
