package util

// Filter 过滤一个切片
func Filter[T any](ss []T, test func(T) bool) (ret []T) {
	for _, s := range ss {
		if test(s) {
			ret = append(ret, s)
		}
	}
	return
}

// Map 将一个切片转换为另一个切片
func Map[T any, R any](ss []T, get func(T) R) (ret []R) {
	for _, s := range ss {
		r := get(s)
		ret = append(ret, r)
	}
	return ret
}

func Unique[T comparable](ss []T) []T {
	seen := make(map[T]bool)
	var ret []T
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			ret = append(ret, s)
		}
	}
	return ret
}
