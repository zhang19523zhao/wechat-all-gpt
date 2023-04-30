package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/wonderivan/logger"
	"github.com/zhang19523zhao/wechat-all-gpt/common"
	"github.com/zhang19523zhao/wechat-all-gpt/config"
	"github.com/zhang19523zhao/wechat-all-gpt/service"
)

func WechatCheck(c *gin.Context) {
	params := new(struct {
		MsgSignature string `form:"msg_signature"`
		Timestamp    string `form:"timestamp"`
		Nonce        string `form:"nonce"`
		Echostr      string `form:"echostr"`
	})

	if err := c.Bind(params); err != nil {
		logger.Error("参数绑定失败: %v\n", err)
	}
	fmt.Println("params:", params)

	wxcpt := &common.WXBizMsgCrypt{}
	switch config.LoadConfig().Type {
	case "wechat_app":
		wechatUrl := config.LoadConfig().WechatApp.WechatUrl
		wxcpt = common.NewWXBizMsgCrypt(wechatUrl.Token, wechatUrl.EncodingAeskey, wechatUrl.Corpid, 1)
	case "wechat_mp":
		c.Writer.WriteString(params.Echostr)
		return
	default:
		logger.Error(fmt.Sprintf("不支持的类型: %s\n", config.LoadConfig().Type))
	}
	res, err := service.WechatCheck.CheckUrl(wxcpt, params.MsgSignature, params.Timestamp, params.Nonce, params.Echostr)
	if err != nil {
		logger.Error(fmt.Sprintf("CheckUrl 失败: %v\n", err))
	}

	c.Writer.WriteString(res)
}
