package log

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/sirupsen/logrus"
)

// MiddlewareRequestLog 创建一个记录请求和响应的中间件
func MiddlewareRequestLog(logResponseContent bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置请求ID
		RequestId(c)

		// 如果需要记录响应内容，创建自定义的响应写入器
		var blw *bodyLogWriter
		if logResponseContent {
			blw = &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
			c.Writer = blw
		}

		// 读取并保存请求体
		var byteBody []byte
		if c.Request.Body != nil {
			byteBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(byteBody))
		} else {
			byteBody = []byte{}
		}

		// 解析请求参数
		reqType := c.ContentType()
		reqBody := map[string]interface{}{}
		if c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "PATCH" {
			if reqType == "application/json" {
				_ = c.ShouldBindBodyWith(&reqBody, binding.JSON)
			} else {
				if err := c.Request.ParseForm(); err == nil {
					for k, vs := range c.Request.Form {
						reqBody[k] = vs
					}
				}
			}
		}

		// 重置请求体
		c.Request.Body = io.NopCloser(bytes.NewBuffer(byteBody))

		// 收集URL参数
		params := map[string]string{}
		for _, p := range c.Params {
			params[p.Key] = p.Value
		}

		// 构建请求日志信息
		req := map[string]interface{}{
			"path":   c.FullPath(),
			"method": c.Request.Method,
			"params": params,
		}
		if c.Request.Method != "GET" {
			req["contentType"] = reqType
			req["body"] = reqBody
		}

		// 创建日志条目（使用 gin.Context 以正确获取 reqId）
		logger := WithFields(c, logrus.Fields{
			"request": req,
		})

		// 处理请求
		c.Next()

		// 记录响应信息
		resType := strings.Split(c.Writer.Header().Get("Content-Type"), ";")[0]
		resBody := "-"
		isJson := strings.Contains(resType, "json")
		if logResponseContent && isJson && blw != nil {
			resBody = blw.body.String()
		}

		status := c.Writer.Status()
		res := map[string]interface{}{
			"contentType": resType,
			"status":      status,
			"size":        c.Writer.Size(),
			"body":        resBody,
		}

		if status < 400 && status >= 300 {
			res["location"] = c.Writer.Header().Get("Location")
		}

		// 根据状态码选择合适的日志级别
		if status >= 500 {
			logger.WithField("response", res).Error()
		} else if status >= 400 {
			logger.WithField("response", res).Warn()
		} else {
			logger.WithField("response", res).Info()
		}
	}
}

// bodyLogWriter 用于捕获响应体的自定义写入器
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w bodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
