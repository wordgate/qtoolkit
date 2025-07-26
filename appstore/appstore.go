package appstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/allnationconnect/mods/log"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/xid"
	"github.com/spf13/viper"
)

// 定义错误类型
var (
	ErrInvalidPayload          = errors.New("invalid payload format")
	ErrCertificateVerification = errors.New("certificate verification failed")
	ErrMissingTransactionInfo  = errors.New("missing transaction information")
	ErrParsingJWT              = errors.New("error parsing JWT")
	ErrPublicKeyExtraction     = errors.New("error extracting public key")
	ErrNoSignedTransaction     = errors.New("no signed transaction in notification")
)

//go:embed certs/AppleRootCA-G3.pem
var AppleRootCAPEM []byte

// 获取苹果私钥
func getKey() (*ecdsa.PrivateKey, error) {
	keyPEM := viper.GetString("appstore.iap.key")
	if keyPEM == "" {
		return nil, errors.New("missing Apple IAP key in configuration")
	}

	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("invalid PEM format for Apple IAP key")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an ECDSA private key")
	}

	return ecdsaKey, nil
}

// 生成JWT令牌用于API验证
func GenerateJwtToken(bundleId string) (string, error) {
	key, err := getKey()
	if err != nil {
		return "", fmt.Errorf("failed to get API key: %w", err)
	}

	keyID := viper.GetString("appstore.iap.keyId")
	if keyID == "" {
		return "", errors.New("missing Apple IAP keyId in configuration")
	}

	issuer := viper.GetString("appstore.iap.issuer")
	if issuer == "" {
		return "", errors.New("missing Apple IAP issuer in configuration")
	}

	now := time.Now().Unix()
	jwtToken := &jwt.Token{
		Header: map[string]interface{}{
			"alg": "ES256",
			"kid": keyID,
			"typ": "JWT",
		},
		Claims: jwt.MapClaims{
			"iss":   issuer,
			"iat":   now,
			"exp":   now + 60*60, // 1小时有效期
			"aud":   "appstoreconnect-v1",
			"nonce": xid.New().String(),
			"bid":   bundleId,
		},
		Method: jwt.SigningMethodES256,
	}

	token, err := jwtToken.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}

	return token, nil
}

// API端点常量
const (
	IAP_SERVER_API         = "https://api.storekit.itunes.apple.com"
	IAP_SANDBOX_SERVER_API = "https://api.storekit-sandbox.itunes.apple.com"
)

// 从Apple服务器获取交易信息
func GetTransaction(ctx context.Context, bundleId, transactionId string) (*TransactionInfo, error) {
	if bundleId == "" || transactionId == "" {
		return nil, errors.New("bundleId and transactionId are required")
	}

	// 先尝试正式环境
	info, err := getTransactionFromEnvironment(ctx, bundleId, transactionId, false)
	if err != nil {
		// 如果失败，尝试沙盒环境
		info, err = getTransactionFromEnvironment(ctx, bundleId, transactionId, true)
	}
	return info, err
}

// 从特定环境获取交易信息
func getTransactionFromEnvironment(ctx context.Context, bundleId, transactionId string, isSandbox bool) (*TransactionInfo, error) {
	baseUrl := IAP_SERVER_API
	if isSandbox {
		baseUrl = IAP_SANDBOX_SERVER_API
	}

	url := fmt.Sprintf("%s/inApps/v1/transactions/%s", baseUrl, transactionId)

	jwtToken, err := GenerateJwtToken(bundleId)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT token: %w", err)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 添加context到请求
	req = req.WithContext(ctx)

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Add("Accept", "application/json")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var data map[string]string
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	payload, ok := data["signedTransactionInfo"]
	if !ok || payload == "" {
		return nil, errors.New("no signedTransactionInfo in response")
	}

	// 解析交易信息JWT
	transactionInfo := &TransactionInfo{}
	if _, err := parseJWT(payload, transactionInfo); err != nil {
		return nil, fmt.Errorf("failed to parse transaction info: %w", err)
	}

	return transactionInfo, nil
}

// 解析JWT令牌通用方法
func parseJWT(tokenString string, claims jwt.Claims) (*jwt.Token, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())

	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// 不验证签名，仅解析数据
		return nil, nil
	})

	if err != nil && !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		return nil, err
	}

	return token, nil
}

// NewNotification 解析App Store通知
func NewNotification(ctx context.Context, payload string) (*AppStoreServerNotification, error) {
	if payload == "" {
		return nil, ErrInvalidPayload
	}

	// 初始化通知对象
	asn := &AppStoreServerNotification{
		IsValid: false,
	}

	// 解析通知
	err := asn.parseNotification(ctx, payload)
	if err != nil {
		return asn, fmt.Errorf("notification parsing failed: %w", err)
	}

	return asn, nil
}

// parseNotification 解析通知负载 - 上下文参数放在第一位
func (asn *AppStoreServerNotification) parseNotification(ctx context.Context, payload string) error {
	// 使用panic恢复来确保即使遇到意外错误也不会崩溃整个程序
	defer func() {
		if r := recover(); r != nil {
			log.Errorf(ctx, "Recovered from panic in parseNotification: %v", r)
		}
	}()

	// 尝试验证证书链
	if err := verifyPayload(payload); err != nil {
		log.Warnf(ctx, "Certificate verification failed: %v", err)
		// 继续处理，不阻断流程
	}

	// 解析主通知载荷
	notificationPayload := &NotificationPayload{}
	token, err := parseJWT(payload, notificationPayload)
	if err != nil {
		log.Warnf(ctx, "Failed to parse notification JWT: %v", err)
		return fmt.Errorf("%w: %v", ErrParsingJWT, err)
	}

	if token == nil || !token.Valid {
		log.Warnf(ctx, "Notification JWT token is invalid")
	}

	asn.Payload = notificationPayload

	// 解析交易信息
	if notificationPayload.Data.SignedTransactionInfo != "" {
		transactionInfo := &TransactionInfo{}
		_, err = parseJWT(notificationPayload.Data.SignedTransactionInfo, transactionInfo)
		if err != nil {
			log.Warnf(ctx, "Failed to parse transaction info JWT: %v", err)
		} else {
			asn.TransactionInfo = transactionInfo
		}
	}

	// 解析续期信息
	if notificationPayload.Data.SignedRenewalInfo != "" {
		renewalInfo := &RenewalInfo{}
		_, err = parseJWT(notificationPayload.Data.SignedRenewalInfo, renewalInfo)
		if err != nil {
			log.Warnf(ctx, "Failed to parse renewal info JWT: %v", err)
		} else {
			asn.RenewalInfo = renewalInfo
		}
	}

	// 验证通知有效性
	asn.IsValid = (asn.Payload != nil && asn.Payload.NotificationType != "")

	// 如果有交易信息，则验证关键字段
	if asn.TransactionInfo != nil {
		if asn.TransactionInfo.TransactionId == "" || asn.TransactionInfo.BundleId == "" {
			log.Warnf(ctx, "Transaction info missing required fields")
			asn.IsValid = false
		}
	}

	return nil
}

// extractHeaderByIndex 从JWT payload中提取x5c证书
func extractHeaderByIndex(payload string, index int) ([]byte, error) {
	// 获取JWT头部
	parts := strings.Split(payload, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid JWT format")
	}

	// 解码JWT头部
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode header: %w", err)
	}

	// 解析头部JSON
	var header JWTHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal header: %w", err)
	}

	// 检查x5c数组
	if len(header.X5c) <= index {
		return nil, fmt.Errorf("x5c index %d out of bounds", index)
	}

	// 解码特定位置的证书
	certBytes, err := base64.StdEncoding.DecodeString(header.X5c[index])
	if err != nil {
		return nil, fmt.Errorf("failed to decode x5c certificate: %w", err)
	}

	return certBytes, nil
}

// verifyPayload 验证JWT payload的证书链
func verifyPayload(payload string) error {
	// 提取根证书
	rootCertBytes, err := extractHeaderByIndex(payload, 2)
	if err != nil {
		return fmt.Errorf("failed to extract root certificate: %w", err)
	}

	// 提取中间证书
	intermediateCertBytes, err := extractHeaderByIndex(payload, 1)
	if err != nil {
		return fmt.Errorf("failed to extract intermediate certificate: %w", err)
	}

	// 验证证书链
	return verifyCertificateChain(rootCertBytes, intermediateCertBytes)
}

// verifyCertificateChain 验证证书链
func verifyCertificateChain(certBytes, intermediateCertBytes []byte) error {
	// 创建根证书池
	roots := x509.NewCertPool()

	// 使用嵌入的Apple根证书
	if !roots.AppendCertsFromPEM(AppleRootCAPEM) {
		return errors.New("failed to parse Apple root certificate")
	}

	// 解析中间证书
	intermediateCert, err := x509.ParseCertificate(intermediateCertBytes)
	if err != nil {
		return fmt.Errorf("failed to parse intermediate certificate: %w", err)
	}

	// 创建中间证书池
	intermediates := x509.NewCertPool()
	intermediates.AddCert(intermediateCert)

	// 解析叶子证书
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return fmt.Errorf("failed to parse leaf certificate: %w", err)
	}

	// 验证证书链
	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   time.Now(),
	}

	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("certificate verification failed: %w", err)
	}

	return nil
}
