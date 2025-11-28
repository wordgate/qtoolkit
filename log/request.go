package log

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/sirupsen/logrus"
)

// MiddlewareRequestLog creates a middleware that logs requests and responses
func MiddlewareRequestLog(logResponseContent bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set request ID
		RequestId(c)

		// Capture response if needed
		var blw *bodyLogWriter
		if logResponseContent {
			blw = &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
			c.Writer = blw
		}

		// Read and save request body
		var byteBody []byte
		if c.Request.Body != nil {
			byteBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(byteBody))
		} else {
			byteBody = []byte{}
		}

		// Parse request params
		reqType := c.ContentType()
		reqBody := map[string]any{}
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

		// Reset request body
		c.Request.Body = io.NopCloser(bytes.NewBuffer(byteBody))

		// Collect URL params
		params := map[string]string{}
		for _, p := range c.Params {
			params[p.Key] = p.Value
		}

		// Build request log
		req := map[string]any{
			"path":   c.FullPath(),
			"method": c.Request.Method,
			"params": params,
		}
		if c.Request.Method != "GET" {
			req["contentType"] = reqType
			req["body"] = reqBody
		}

		// Create log entry
		entry := WithFields(c, logrus.Fields{"request": req})

		// Process request
		c.Next()

		// Log response
		resType := strings.Split(c.Writer.Header().Get("Content-Type"), ";")[0]
		resBody := "-"
		if logResponseContent && strings.Contains(resType, "json") && blw != nil {
			resBody = blw.body.String()
		}

		status := c.Writer.Status()
		res := map[string]any{
			"contentType": resType,
			"status":      status,
			"size":        c.Writer.Size(),
			"body":        resBody,
		}

		if status >= 300 && status < 400 {
			res["location"] = c.Writer.Header().Get("Location")
		}

		// Log with appropriate level
		if status >= 500 {
			entry.WithField("response", res).Error()
		} else if status >= 400 {
			entry.WithField("response", res).Warn()
		} else {
			entry.WithField("response", res).Info()
		}
	}
}

// bodyLogWriter captures response body
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
