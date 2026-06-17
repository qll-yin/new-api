package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TokenOrUserAuth())
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.POST("/video/generations", controller.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", controller.RelayTaskFetch)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)
		klingV1Router.POST("/videos/image2video", controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask)
	}

	// 阿里云百炼 DashScope 原生视频生成路径 - 客户端只需把 base_url 换成本平台即可。
	// 提交与查询两端均返回 dashscope 原生格式，现有 dashscope 代码改 base_url 即可闭环。
	dashScopeSubmitGroup := router.Group("/api/v1/services/aigc/video-generation")
	dashScopeSubmitGroup.Use(middleware.RouteTag("relay"))
	dashScopeSubmitGroup.Use(middleware.HappyHorseRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		dashScopeSubmitGroup.POST("/video-synthesis", controller.RelayTask)
	}

	dashScopeImage2VideoSubmitGroup := router.Group("/api/v1/services/aigc/image2video")
	dashScopeImage2VideoSubmitGroup.Use(middleware.RouteTag("relay"))
	dashScopeImage2VideoSubmitGroup.Use(middleware.HappyHorseRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		dashScopeImage2VideoSubmitGroup.POST("/video-synthesis", controller.RelayTask)
	}

	dashScopeFetchGroup := router.Group("/api/v1/tasks")
	dashScopeFetchGroup.Use(middleware.RouteTag("relay"))
	dashScopeFetchGroup.Use(middleware.HappyHorseFetchConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		dashScopeFetchGroup.GET("/:task_id", controller.RelayTaskFetch)
	}
}
