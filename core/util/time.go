package util

import (
	"strings"
	"time"
)

func ExpiredTimeAdd(t time.Time, d time.Duration) time.Time {
	if t.Before(time.Now()) {
		return time.Now().Add(d)
	}
	return t.Add(d)
}

func ExpiredTimeRecount(originExpiredTime time.Time, originUserCount int64, toUserCount int64) time.Time {
	now := time.Now()
	if originExpiredTime.Before(now) {
		return originExpiredTime
	}
	// 使用戶originExpiredTime 到現在的秒數作為originSeconds
	// seconds * originUserCount / toUserCount 作為 toSeconds
	// 返回 now + toSeconds 的時間
	originSeconds := originExpiredTime.Sub(now).Seconds()
	toSeconds := originSeconds * float64(originUserCount) / float64(toUserCount)
	return now.Add(time.Duration(toSeconds) * time.Second)
}

// ParseTime 将时间字符串解析为 time.Time
// 支持格式：2006-01-02 15:04:05
// loc 参数用于指定时区，如果为 nil 则使用本地时区
func ParseTime(timeStr string, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}

	// 标准化时间字符串
	timeStr = strings.TrimSpace(timeStr)

	t, _ := time.ParseInLocation("2006-01-02 15:04:05", timeStr, loc)
	return t
}

// ParseDate 将时间字符串解析为 time.Time
// 支持格式：2006-01-02
// loc 参数用于指定时区，如果为 nil 则使用本地时区
func ParseDate(dateStr string, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}

	// 标准化时间字符串
	dateStr = strings.TrimSpace(dateStr)

	t, _ := time.ParseInLocation("2006-01-02", dateStr, loc)
	return t
}
