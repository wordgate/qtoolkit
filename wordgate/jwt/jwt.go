package jwt

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JWTConfig JWT配置
type JWTConfig struct {
	Secret        string   `json:"secret"`         // JWT密钥
	ExpiresIn     int      `json:"expires_in"`     // 访问令牌过期时间（秒）
	RefreshExpire int      `json:"refresh_expire"` // 刷新令牌过期时间（秒）
	AllowDomains  []string `json:"allow_domains"`  // 允许的回调域名白名单（例如：["example.com", "*.example.com"]）
}

// JWTToken JWT令牌信息
type JWTToken struct {
	Token     string `json:"token"`      // JWT令牌
	ExpiredAt int64  `json:"expired_at"` // 过期时间（绝对时间戳，秒）
}

// JWTService JWT服务
type JWTService struct {
	config JWTConfig
}

// NewJWTService 创建JWT服务
func NewJWTService(config JWTConfig) *JWTService {
	return &JWTService{
		config: config,
	}
}

// GenerateToken 生成JWT访问令牌
func (s *JWTService) GenerateToken(uuid string, role string) (*JWTToken, error) {
	now := time.Now()
	expiresIn := time.Duration(s.config.ExpiresIn) * time.Second
	expiredAt := now.Add(expiresIn).Unix()

	claims := jwt.MapClaims{
		"sub":  uuid,       // 用户UUID
		"role": role,       // 用户角色
		"iat":  now.Unix(), // 签发时间
		"exp":  expiredAt,  // 过期时间
		"typ":  "access",   // 令牌类型
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.config.Secret))
	if err != nil {
		return nil, err
	}

	return &JWTToken{
		Token:     tokenString,
		ExpiredAt: expiredAt,
	}, nil
}

// GenerateRefreshToken 生成刷新令牌
func (s *JWTService) GenerateRefreshToken(uuid string) (string, error) {
	now := time.Now()
	refreshExpire := time.Duration(s.config.RefreshExpire) * time.Second

	claims := jwt.MapClaims{
		"sub": uuid,                          // 用户UUID
		"iat": now.Unix(),                    // 签发时间
		"exp": now.Add(refreshExpire).Unix(), // 过期时间
		"typ": "refresh",                     // 令牌类型
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.Secret))
}

// ValidateToken 验证JWT令牌
func (s *JWTService) ValidateToken(tokenString string) (*jwt.Token, error) {
	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.config.Secret), nil
	})
}

// JwtUserID 从请求中获取用户UUID
func (s *JWTService) JwtUserID(c *gin.Context) string {
	token := c.GetHeader("Authorization")
	if token == "" {
		return ""
	}

	// 去掉Bearer前缀
	token = strings.TrimPrefix(token, "Bearer ")

	// 解析令牌
	jwtToken, err := s.ValidateToken(token)
	if err != nil || !jwtToken.Valid {
		return ""
	}

	// 从令牌中获取用户信息
	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}

	// 检查令牌类型
	tokenType, ok := claims["typ"].(string)
	if !ok || tokenType != "access" {
		return ""
	}
	return claims["sub"].(string)
}


// JwtUserInfo 从请求中获取用户UUID和角色
func (s *JWTService) JwtUserInfo(c *gin.Context) (uuid string, role string) {
	token := c.GetHeader("Authorization")
	if token == "" {
		return "", ""
	}

	// 去掉Bearer前缀
	token = strings.TrimPrefix(token, "Bearer ")

	// 解析令牌
	jwtToken, err := s.ValidateToken(token)
	if err != nil || !jwtToken.Valid {
		return "", ""
	}

	// 从令牌中获取用户信息
	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		return "", ""
	}

	// 检查令牌类型
	tokenType, ok := claims["typ"].(string)
	if !ok || tokenType != "access" {
		return "", ""
	}

	uuid, _ = claims["sub"].(string)
	role, _ = claims["role"].(string)
	return uuid, role
}
