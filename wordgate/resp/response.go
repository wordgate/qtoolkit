package resp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ErrorCode int

const (
	ErrorInvalidArgument    ErrorCode = 422 // 无效参数
	ErrorNotFound           ErrorCode = 404 // 未找到
	ErrorNotLogin           ErrorCode = 401 // 未登录
	ErrorForbidden          ErrorCode = 403 // 权限不足
	ErrorConflict           ErrorCode = 409 // 冲突
	ErrorInvalidOperation   ErrorCode = 400 // 无效操作
	ErrorSystemError        ErrorCode = 500 // 系统错误
	ErrorServiceUnavailable ErrorCode = 503 // 服务不可用
)

type Response struct {
	Code    int         `json:"code"`              // 返回码，0为成功，非0为失败
	Message string      `json:"message,omitempty"` // 错误信息
	Data    interface{} `json:"data,omitempty"`    // 响应数据
}

type Pagination struct {
	Page  int   `json:"page"`  // 当前页码
	Limit int   `json:"limit"` // 每页数量
	Total int64 `json:"total"` // 总记录数
}

type ListResult struct {
	Items      interface{}            `json:"items"`
	Pagination Pagination             `json:"pagination"`
	Extra      map[string]interface{} `json:"extra,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	if data == nil {
		data = gin.H{}
	}
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Data: data,
	})
}

func List(c *gin.Context, items interface{}, pagination Pagination) {
	c.JSON(http.StatusOK, Response{
		Code: 0,
		Data: ListResult{
			Items:      items,
			Pagination: pagination,
		},
	})
}

func Error(c *gin.Context, code ErrorCode, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    int(code),
		Message: message,
	})
}
