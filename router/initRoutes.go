package router

import (
	"github.com/gin-gonic/gin"
	"github.com/zhang19523zhao/wechat-all-gpt/controller"
)

func InitRoutes(r *gin.Engine) {
	r.GET("/wechat/check", controller.WechatCheck)
	r.POST("/wechat/check", controller.TailkWeixin)
}
