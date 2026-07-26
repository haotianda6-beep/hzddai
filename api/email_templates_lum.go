package api

import (
	"fmt"
	"strings"
)

// 萤火主题 HTML 邮件（内联样式，兼容常见客户端）
func lumVerificationEmailHTML(brand, headline, code, footNote string) string {
	code = htmlEscape(code)
	brand = htmlEscape(brand)
	headline = htmlEscape(headline)
	footNote = htmlEscape(footNote)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background-color:#0c0e12;font-family:'Segoe UI',Roboto,'Helvetica Neue',Arial,'Noto Sans SC',sans-serif;">
<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="background-color:#0c0e12;padding:32px 16px;">
<tr><td align="center">
<table role="presentation" width="100%%" style="max-width:520px;border-collapse:collapse;">
<tr><td style="padding:0 0 20px 0;text-align:center;">
<span style="display:inline-block;font-size:13px;font-weight:700;letter-spacing:0.25em;color:#cafd00;text-transform:uppercase;">%s</span>
</td></tr>
<tr><td style="background:linear-gradient(135deg,rgba(29,32,37,0.98) 0%%,rgba(17,19,24,0.98) 100%%);border:1px solid rgba(202,253,0,0.35);border-radius:20px;box-shadow:0 0 0 1px rgba(202,253,0,0.08),0 24px 48px rgba(0,0,0,0.45);overflow:hidden;">
<table role="presentation" width="100%%" cellspacing="0" cellpadding="0">
<tr><td style="padding:28px 28px 8px 28px;">
<p style="margin:0;font-size:18px;font-weight:700;color:#f6f6fc;line-height:1.4;">%s</p>
<p style="margin:12px 0 0 0;font-size:14px;color:#aaabb0;line-height:1.6;">请使用下方验证码完成操作。验证码 <strong style="color:#cafd00;">10 分钟</strong>内有效。</p>
</td></tr>
<tr><td style="padding:8px 28px 28px 28px;text-align:center;">
<div style="display:inline-block;padding:18px 36px;border-radius:16px;background-color:#cafd00;box-shadow:0 0 20px rgba(202,253,0,0.25);">
<span style="font-size:32px;font-weight:800;letter-spacing:0.45em;color:#0c0e12;font-family:ui-monospace,Menlo,Consolas,monospace;">%s</span>
</div>
</td></tr>
<tr><td style="padding:0 28px 24px 28px;border-top:1px solid rgba(70,72,77,0.5);">
<p style="margin:16px 0 0 0;font-size:12px;color:#84878f;line-height:1.6;">%s</p>
<p style="margin:12px 0 0 0;font-size:11px;color:#5c5e64;">此为系统邮件，请勿直接回复。如非本人操作，请忽略并妥善保管账户安全。</p>
</td></tr>
</table>
</td></tr>
<tr><td style="padding:20px 8px 0;text-align:center;">
<p style="margin:0;font-size:11px;color:#46484d;">© %s · AI 量化</p>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`, brand, headline, code, footNote, brand)
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
