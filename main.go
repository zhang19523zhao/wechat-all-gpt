package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/zhang19523zhao/wechat-all-gpt/config"
	"github.com/zhang19523zhao/wechat-all-gpt/router"
)

func main() {
	r := gin.Default()
	router.InitRoutes(r)
	r.Run(fmt.Sprintf(":%s", config.LoadConfig().Port))
}
