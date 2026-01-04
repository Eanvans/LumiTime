package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	subtube "subtuber-services/protos"

	"github.com/gin-gonic/gin"
	cache "github.com/patrickmn/go-cache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	EmailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	codeCache  = cache.New(10*time.Minute, 1*time.Minute)
)

type sendCodeRequest struct {
	Email string `json:"email" binding:"required"`
}

type verifyRequest struct {
	Email string `json:"email" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

type userPreferences struct {
	Language           string `json:"language"`
	Timezone           string `json:"timezone"`
	EmailNotifications bool   `json:"emailNotifications"`
}

type userModel struct {
	UserId       string          `json:"userId"`
	Email        string          `json:"email"`
	DisplayName  string          `json:"displayName"`
	RegisteredAt time.Time       `json:"registeredAt"`
	LastLoginAt  time.Time       `json:"lastLoginAt"`
	Preferences  userPreferences `json:"preferences"`
}

// RegisterAuthRoutes registers authentication-related endpoints under /api/auth
func RegisterAuthRoutes(r *gin.Engine) {
	g := r.Group("/api/auth")
	g.POST("/send-code", sendCodeHandler)
	g.POST("/verify", verifyHandler)
}

func sendCodeHandler(c *gin.Context) {
	var req sendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "message": "invalid request"})
		return
	}

	email := strings.TrimSpace(req.Email)
	if !EmailRegex.MatchString(email) {
		c.JSON(400, gin.H{"success": false, "message": "无效的邮箱地址。"})
		return
	}

	code := generateNumericCode(6)
	key := "login:code:" + strings.ToLower(email)
	codeCache.Set(key, code, 10*time.Minute)

	// ensure App_Data exists and append to emails.log for debugging
	baseDir := "App_Data"
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		log.Printf("failed create App_Data: %v", err)
	} else {
		logLine := fmt.Sprintf("%s\t%s\t%s\n", time.Now().UTC().Format(time.RFC3339Nano), email, code)
		_ = os.WriteFile(filepath.Join(baseDir, "emails.log"), []byte(logLine), 0o644)
		// Append instead of overwrite
		f, err := os.OpenFile(filepath.Join(baseDir, "emails.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.WriteString(logLine)
			_ = f.Close()
		}
	}

	// Try to send email if SMTP configured (use injected smtpCfg)
	smtpHost := smtpCfg.Host
	if smtpHost != "" {
		smtpPort := smtpCfg.Port
		if smtpPort == "" {
			smtpPort = "25"
		}
		smtpUser := smtpCfg.User
		smtpPass := smtpCfg.Pass
		from := smtpCfg.From
		if from == "" {
			if smtpUser != "" {
				from = smtpUser
			} else {
				from = "no-reply@localhost"
			}
		}

		subject := "您的登录验证码"
		body := fmt.Sprintf("您的验证码为：%s（有效期 10 分钟）", code)

		msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", from, email, subject, body)

		addr := smtpHost + ":" + smtpPort
		auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

		// if send fails, log and write to emails-errors.log
		if err := SendMailWithTLS(addr, auth, from, []string{email}, []byte(msg)); err != nil {
			log.Printf("smtp send failed: %v", err)
			_ = appendErrorLog("emails-errors.log", fmt.Sprintf("%s\tSMTP_ERROR\t%s\tTo:%s\tErr:%v\n", time.Now().UTC().Format(time.RFC3339Nano), addr, email, err))
		} else {
			log.Printf("sent email to %s", email)
		}
	} else {
		log.Printf("SMTP not configured, code logged for %s", email)
	}

	c.JSON(200, gin.H{"success": true, "message": "验证码已发送（如果未收到请检查垃圾邮件或联系管理员）。"})
}

// Dial return a smtp client
func Dial(addr string) (*smtp.Client, error) {
	conn, err := tls.Dial("tcp", addr, nil)
	if err != nil {
		log.Println("tls.Dial Error:", err)
		return nil, err
	}

	host, _, _ := net.SplitHostPort(addr)
	return smtp.NewClient(conn, host)
}

// SendMailWithTLS send email with tls
func SendMailWithTLS(addr string, auth smtp.Auth, from string,
	to []string, msg []byte) (err error) {
	//create smtp client
	c, err := Dial(addr)
	if err != nil {
		log.Println("Create smtp client error:", err)
		return err
	}
	defer c.Close()
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); ok {
			if err = c.Auth(auth); err != nil {
				log.Println("Error during AUTH", err)
				return err
			}
		}
	}
	if err = c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err = c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

func verifyHandler(c *gin.Context) {
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "message": "invalid request"})
		return
	}

	email := strings.TrimSpace(req.Email)
	code := strings.TrimSpace(req.Code)

	if !EmailRegex.MatchString(email) {
		c.JSON(400, gin.H{"success": false, "message": "无效的邮箱地址。"})
		return
	}

	if code == "" {
		c.JSON(400, gin.H{"success": false, "message": "请输入验证码。"})
		return
	}

	key := "login:code:" + strings.ToLower(email)
	v, found := codeCache.Get(key)
	if !found || v == nil || v.(string) != code {
		c.JSON(400, gin.H{"success": false, "message": "验证码错误或已过期。请重新发送验证码并重试。"})
		return
	}

	safe := computeSha256Hex(strings.ToLower(email))
	baseDir := filepath.Join("App_Data")
	userDir := filepath.Join(baseDir, safe)
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		log.Printf("failed to create user dir: %v", err)
	}

	userModelPath := filepath.Join(userDir, "user.json")
	now := time.Now().UTC()
	var user userModel
	if _, err := os.Stat(userModelPath); os.IsNotExist(err) {
		displayName := strings.Split(email, "@")[0]
		user = userModel{
			UserId:       safe,
			Email:        email,
			DisplayName:  displayName,
			RegisteredAt: now,
			LastLoginAt:  now,
			Preferences:  userPreferences{Language: "zh-CN", Timezone: "Asia/Shanghai", EmailNotifications: true},
		}
		// save
		if b, err := json.MarshalIndent(user, "", "  "); err == nil {
			_ = os.WriteFile(userModelPath, b, 0o644)
		}
	} else {
		// load and update
		b, err := os.ReadFile(userModelPath)
		if err == nil {
			_ = json.Unmarshal(b, &user)
		}
		if user.UserId == "" {
			user.UserId = safe
			user.Email = email
			user.DisplayName = strings.Split(email, "@")[0]
			user.RegisteredAt = now
		}
		user.LastLoginAt = now
		if b, err := json.MarshalIndent(user, "", "  "); err == nil {
			_ = os.WriteFile(userModelPath, b, 0o644)
		}
	}

	// write email.txt for compatibility
	_ = os.WriteFile(filepath.Join(userDir, "email.txt"), []byte(email), 0o644)

	// set cookie with user info (JSON)
	if b, err := json.Marshal(user); err == nil {
		// maxAge in seconds; set long expiration (10 years)
		maxAge := 10 * 365 * 24 * 60 * 60
		c.SetCookie("UserInfo", string(b), maxAge, "/", "", true, true)
	}

	// remove cached code
	codeCache.Delete(key)

	// 返回用户信息并已写入 Cookie（不进行跳转，前端负责处理）
	c.JSON(200, gin.H{"success": true, "message": "登录成功", "user": user})

	// asynchronously notify data layer to create user via gRPC
	sendCreateUserToRPC(user)
}

// sendCreateUserToRPC dials the UserProfileRpc service and calls CreateUser asynchronously.
// Address is taken from USER_RPC_ADDR env var, defaulting to localhost:50051.
func sendCreateUserToRPC(u userModel) {
	go func(user userModel) {
		addr := os.Getenv("USER_RPC_ADDR")
		if addr == "" {
			addr = "localhost:50051"
		}

		// dial without blocking; use a short timeout for the CreateUser RPC itself
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("failed to dial user rpc %s: %v", addr, err)
			return
		}
		defer conn.Close()

		client := subtube.NewUserProfileRpcClient(conn)
		req := &subtube.CreateUserRequest{
			UserHash:         user.UserId,
			Email:            user.Email,
			MaxTrackingLimit: 5,
		}

		callCtx, callCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer callCancel()
		if _, err := client.CreateUser(callCtx, req); err != nil {
			log.Printf("CreateUser RPC failed for %s: %v", user.Email, err)
		} else {
			log.Printf("CreateUser RPC succeeded for %s", user.Email)
		}
	}(u)
}

func generateNumericCode(digits int) string {
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		// fallback
		return fmt.Sprintf("%0*d", digits, time.Now().UnixNano()%int64(1<<30))
	}
	format := fmt.Sprintf("%%0%dd", digits)
	return fmt.Sprintf(format, n.Int64())
}

func computeSha256Hex(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

func appendErrorLog(filename, line string) error {
	baseDir := "App_Data"
	_ = os.MkdirAll(baseDir, 0o755)
	f, err := os.OpenFile(filepath.Join(baseDir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}
