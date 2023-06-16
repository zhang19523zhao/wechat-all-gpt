package model

import "encoding/xml"

// WechatApp  企业微信应用
type WechatApp struct {
	Enable    bool      `json:"enable"`
	WechatUrl WechatUrl `json:"wechat_url"`
}

// WechatMp  微信公众号
type WechatMp struct {
	Enable    bool      `json:"enable"`
	WechatUrl WechatUrl `json:"wechat_url"`
	Limit     int       `json:"limit"`
}

type WechatUrl struct {
	Token          string `json:"token"`
	Corpid         string `json:"corpid"`
	EncodingAeskey string `json:"encoding_aeskey"`
	Corpsecret     string `json:"corpsecret"`
}

// WeixinUserAskMsg 企业微信应用接收消息格式
type WeixinUserAskMsg struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   string `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	MsgId        string `xml:"MsgId"`
	AgentID      string `xml:"AgentID"`
}

// ReplyMsg 企业微信应用发送消息格式
type ReplyMsg struct {
	Touser  string `json:"touser,omitempty"`
	Toparty string `json:"toparty,omitempty"`
	Totag   string `json:"totag,omitempty"`
	Msgtype string `json:"msgtype,omitempty"`
	Agentid string `json:"agentid,omitempty"`
	Text    struct {
		Content string `json:"content,omitempty"`
	} `json:"text,omitempty"`
}

// ReplyImage 企业微信应用发送图片消息格式
type ReplyImage struct {
	Touser  string `json:"touser,omitempty"`
	Toparty string `json:"toparty,omitempty"`
	Totag   string `json:"totag,omitempty"`
	Msgtype string `json:"msgtype,omitempty"`
	Agentid string `json:"agentid,omitempty"`
	Image   struct {
		MediaId string `json:"media_id,omitempty"`
	} `json:"image,omitempty"`
}

// UploadTmpResp 企业微信临时素材resp
type UploadTmpResp struct {
	Errcode   string `json:"errcode"`
	Errmsg    string `json:"errmsg"`
	Type      string `json:"type"`
	MediaId   string `json:"media_id"`
	CreatedAt string `json:"created_at"`
}

// WeixinMapAskMsg 微信公众号用户消息
type WeixinMapAskMsg struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   string `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	MsgId        string `xml:"MsgId"`
	MsgDataId    string `xml:"MsgDataId"`
	Idx          string `xml:"Idx"`
}

type ReplyMp struct {
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int      `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	XMLName      xml.Name `xml:"xml"`
}

// ReplyAll 包含所有返回类型
type ReplyAll struct {
	RepText  ReplyMsg
	RepImage ReplyImage
}
