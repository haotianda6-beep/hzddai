package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// handleSendRegisterEmailCode 发送注册验证码（邮箱尚未注册即可）
func (s *Server) handleSendRegisterEmailCode(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "请填写有效邮箱")
		return
	}

	email := req.Email
	if _, err := s.store.User().GetByEmail(email); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "该邮箱已注册"})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		SafeInternalError(c, "查询邮箱失败", err)
		return
	}

	if ok, wait := emailOTPResendAllowed(purposeRegister(), email); !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":       fmt.Sprintf("发送太频繁，请 %.0f 秒后再试", wait.Seconds()),
			"retry_after": int(wait.Seconds()),
		})
		return
	}

	code, err := randomDigits6()
	if err != nil {
		SafeInternalError(c, "生成验证码失败", err)
		return
	}

	emailOTPSave(purposeRegister(), email, code)
	brand := smtpBrand()
	subject := fmt.Sprintf("【%s】注册验证码", brand)
	plain := fmt.Sprintf("您的注册验证码是：%s\n\n10 分钟内有效。如非本人操作请忽略本邮件。", code)
	html := lumVerificationEmailHTML(
		brand,
		"欢迎注册「"+brand+"」",
		code,
		"您正在使用邮箱 "+email+" 注册新账号。",
	)
	sent, err := sendEmailSMTP(email, subject, plain, html)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "邮件发送失败，请检查 SMTP 配置或稍后重试"})
		return
	}

	resp := gin.H{"message": "验证码已发送"}
	if !sent {
		resp["email_sent"] = false
		resp["hint"] = "服务器未配置发信邮箱，验证码已记录在服务端日志，仅供开发调试"
	} else {
		resp["email_sent"] = true
	}
	c.JSON(http.StatusOK, resp)
}

// handleSendResetPasswordEmailCode 向已注册用户发送重置密码验证码
func (s *Server) handleSendResetPasswordEmailCode(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SafeBadRequest(c, "请填写有效邮箱")
		return
	}

	email := req.Email
	if _, err := s.store.User().GetByEmail(email); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "该邮箱未注册"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}

	if ok, wait := emailOTPResendAllowed(purposeReset(), email); !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":       fmt.Sprintf("发送太频繁，请 %.0f 秒后再试", wait.Seconds()),
			"retry_after": int(wait.Seconds()),
		})
		return
	}

	code, err := randomDigits6()
	if err != nil {
		SafeInternalError(c, "生成验证码失败", err)
		return
	}

	emailOTPSave(purposeReset(), email, code)
	brand := smtpBrand()
	subject := fmt.Sprintf("【%s】重置密码验证码", brand)
	plain := fmt.Sprintf("您正在重置「%s」账户登录密码。\n验证码：%s\n\n10 分钟内有效。如非本人操作请立即修改密码并联系管理员。", brand, code)
	html := lumVerificationEmailHTML(
		brand,
		"重置登录密码",
		code,
		"您正在使用邮箱 "+email+" 申请重置密码。",
	)
	sent, err := sendEmailSMTP(email, subject, plain, html)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "邮件发送失败，请检查 SMTP 配置或稍后重试"})
		return
	}

	resp := gin.H{"message": "验证码已发送"}
	if !sent {
		resp["email_sent"] = false
		resp["hint"] = "服务器未配置发信邮箱，验证码已记录在服务端日志，仅供开发调试"
	} else {
		resp["email_sent"] = true
	}
	c.JSON(http.StatusOK, resp)
}
