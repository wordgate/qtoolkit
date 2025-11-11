package qtoolkit

/*
邀请码容量对照表（约等于最大可编码ID+1）

| 长度 | 区分大小写（55种字符） | 不区分大小写（32种字符） |
|------|----------------------|-------------------------|
| 4    | 9,150,625            | 1,048,576               |
| 5    | 503,284,375          | 33,554,432              |
| 6    | 27,680,640,625       | 1,073,741,824           |
| 7    | 1,522,435,234,375    | 34,359,738,368          |
| 8    | 83,734,937,890,625   | 1,099,511,627,776       |

说明：
- 区分大小写：CHARSET = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz"（共55种字符）
- 不区分大小写：CHARSET_CASE_INSENSITIVE = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"（共32种字符）
- 实际可用数量 = pow(字符集长度, 邀请码长度)
*/

import (
	"errors"
	"math/rand"
	"strings"
)

const (
	// 此处绝对不可再变更，因为已经有业务使用
	CHARSET = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz"
	// 只包含大写字母和数字，去除易混淆字符（0, 1, O, I, L）
	CHARSET_CASE_INSENSITIVE = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

type NumEncoder struct {
	length        int
	coprime       int
	decodeFactor  uint16
	maxSupport    uint64
	charset       string
	caseSensitive bool
	base          uint64 // 字符集长度（区分大小写时为55，不区分时为32）
}

// NewNumEncoder 创建一个数字编码器
// length 编码长度
// seed 随机种子
// caseSensitive 是否区分大小写
func NewNumEncoder(length uint8, seed int64, caseSensitive bool) (*NumEncoder, error) {
	var charset string
	if caseSensitive {
		charset = CHARSET
	} else {
		charset = CHARSET_CASE_INSENSITIVE
	}
	shuffledCharset := shuffleCharset(charset, seed)
	base := uint64(len(charset)) // 根据实际字符集长度设置 base

	encoder := &NumEncoder{
		length:        int(length),
		coprime:       int(minCoprime(uint64(length))),
		decodeFactor:  uint16(len(charset)) * uint16(length),
		maxSupport:    pow(uint64(len(charset)), uint64(length)) - 1,
		charset:       shuffledCharset,
		caseSensitive: caseSensitive,
		base:          base, // 初始化实例的 base 字段
	}

	return encoder, nil
}

func (g *NumEncoder) MaxSupportID() uint64 {
	return g.maxSupport
}

// Encode 通过id获取指定邀请码（进制法+扩散+混淆）
func (g *NumEncoder) Encode(id uint64) (string, error) {
	if id > g.maxSupport {
		return "", errors.New("id out of range")
	}

	idx := make([]uint16, g.length)

	// 扩散
	for i := 0; i < g.length; i++ {
		idx[i] = uint16(id % g.base) // 使用实例的 base
		idx[i] = (idx[i] + uint16(i)*idx[0]) % uint16(g.base) // 使用实例的 base
		id /= g.base // 使用实例的 base
	}

	// 混淆
	var buf strings.Builder
	buf.Grow(g.length)
	for i := 0; i < g.length; i++ {
		n := i * g.coprime % g.length
		buf.WriteByte(g.charset[idx[n]])
	}

	return buf.String(), nil
}

// Decode 通过邀请码反推id
func (g *NumEncoder) Decode(code string) uint64 {
	if !g.caseSensitive {
		code = strings.ToUpper(code)
	}
	var idx = make([]uint16, g.length)
	for i, c := range code {
		idx[i*g.coprime%g.length] = uint16(strings.IndexRune(g.charset, c)) // 反推下标数组
	}

	var id uint64
	for i := g.length - 1; i >= 0; i-- {
		id *= g.base // 使用实例的 base
		idx[i] = (idx[i] + g.decodeFactor - idx[0]*uint16(i)) % uint16(g.base) // 使用实例的 base
		id += uint64(idx[i])
	}

	return id
}

// 求uint64类型n的最小互质数
func minCoprime(n uint64) uint64 {
	// 如果n是1，那么最小互质数是2
	if n == 1 {
		return 2
	}
	// 从2开始遍历，找到第一个和n互质的数
	for i := uint64(2); i < n; i++ {
		// 如果i和n的最大公约数是1，那么i就是最小互质数
		if isCoprime(i, n) {
			return i
		}
	}
	// 如果没有找到，那么返回n+1，因为n+1一定和n互质
	return n + 1
}

// 判断两个数是否互质
func isCoprime(n, m uint64) bool {
	// 求最大公因数
	return gcd(n, m) == 1
}

// 辗转相除法求最大公因数
func gcd(n, m uint64) uint64 {
	if m == 0 {
		return n
	}
	return gcd(m, n%m)
}

// 求n的m次方
func pow(n, m uint64) uint64 {
	sum := n
	for i := uint64(1); i < m; i++ {
		sum *= n
	}
	return sum
}

func shuffleCharset(charset string, seed int64) string {
	// 将字符串转换为 rune 切片以便打乱
	chars := []rune(charset)

	// 使用提供的 seed 创建随机数生成器
	r := rand.New(rand.NewSource(seed))

	// Fisher-Yates 洗牌算法
	for i := len(chars) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		chars[i], chars[j] = chars[j], chars[i]
	}

	return string(chars)
}
