package util

import (
	"strings"
)

func SecretEmail(email string) string {
	if email == "" {
		return ""
	}

	arr := strings.Split(email, "@")
	if len(arr) != 2 {
		return email // 如果不是有效的邮箱格式，直接返回原值
	}

	prefix := arr[0]
	if len(prefix) <= 2 {
		// 如果前缀太短，就用 * 填充
		return prefix + "@" + arr[1]
	}

	// 原来的逻辑
	return prefix[:2] + "***@" + arr[1]
}
