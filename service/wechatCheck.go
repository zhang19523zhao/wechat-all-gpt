package service

import (
	"errors"
	"fmt"
	"github.com/wonderivan/logger"
	"github.com/zhang19523zhao/wechat-all-gpt/common"
)

type wechatCheck struct{}

var WechatCheck wechatCheck

func (w *wechatCheck) CheckUrl(wxcpt *common.WXBizMsgCrypt, msg_signature, timestamp, nonce, echostr string) (string, error) {
	echoStr, cryptErr := wxcpt.VerifyURL(msg_signature, timestamp, nonce, echostr)
	if cryptErr != nil {
		logger.Info("VerifyUrl 失败: %v\n", cryptErr)
		return "", errors.New(fmt.Sprintf("VerifyUrl 失败: %v\n", cryptErr))
	}
	res := string(echoStr)

	return res, nil
}
