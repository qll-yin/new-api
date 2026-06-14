package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
)

// HappyHorseRequestConvert 将阿里云百炼 DashScope 原生视频生成请求转换为内部统一任务格式。
//
// 原生请求体（DashScope）：
//
//	{ "model": "happyhorse-1.0-t2v",
//	  "input": { "prompt": "...", "media": [...] },
//	  "parameters": { "resolution": "720P", "ratio": "16:9", "duration": 5 } }
//
// 转换后（与 /v1/video/generations 统一入口收敛到同一 metadata 结构）：
//
//	{ "model": "...", "prompt": "...",
//	  "metadata": { "media": [...], "parameters": {...} } }
//
// 响应仍返回 new-api 统一（OpenAI 视频）格式。
func HappyHorseRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}

		model, _ := originalReq["model"].(string)

		var prompt string
		var media interface{}
		if input, ok := originalReq["input"].(map[string]interface{}); ok {
			prompt, _ = input["prompt"].(string)
			media = input["media"]
		}

		metadata := map[string]interface{}{}
		if media != nil {
			metadata["media"] = media
		}
		if parameters, ok := originalReq["parameters"]; ok {
			metadata["parameters"] = parameters
		}

		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": metadata,
		}

		jsonData, err := common.Marshal(unifiedReq)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to marshal request body")
			return
		}

		// 重写请求体与路径，复用统一任务管线（计费/渠道选择/轮询均自动生效）
		common.ReplaceRequestBodyReusable(c, jsonData)
		c.Request.URL.Path = "/v1/video/generations"
		// 标记为 DashScope 原生调用：响应需返回 dashscope 原生格式
		c.Set("dashscope_native", true)

		// 无 media（纯文生视频）时标记为 textGenerate
		if media == nil {
			c.Set("action", constant.TaskActionTextGenerate)
		}

		c.Next()
	}
}

// HappyHorseFetchConvert 处理阿里云百炼 DashScope 原生任务查询路径
// （GET /api/v1/tasks/:task_id），将其重写为内部统一查询路径并标记为原生格式，
// 使响应返回 dashscope 原生结构。
func HappyHorseFetchConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		taskID := c.Param("task_id")
		if taskID == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "task_id is required")
			return
		}
		c.Set("task_id", taskID)
		c.Set("dashscope_native", true)
		// 重写为内部统一查询路径，复用已验证的 fetch 逻辑与 Distribute 视频分支
		c.Request.URL.Path = "/v1/video/generations/" + taskID
		c.Next()
	}
}
