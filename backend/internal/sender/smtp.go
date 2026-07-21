package sender

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// SMTPProvider 通过 SMTP 发送邮件验证码，是一个可直接使用的真实参考实现。
//
// 注意：标准库 net/smtp 的 SendMail 不支持 context 取消，ctx 仅用于接口一致性。
// 生产高并发场景可替换为带连接池/超时控制的邮件库。
type SMTPProvider struct {
	addr string // host:port
	auth smtp.Auth
	from string
}

// NewSMTPProvider 构造 SMTP 邮件 Provider。username 为空时不做鉴权（适合内网中继）。
func NewSMTPProvider(host string, port int, username, password, from string) *SMTPProvider {
	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}
	return &SMTPProvider{
		addr: net.JoinHostPort(host, strconv.Itoa(port)),
		auth: auth,
		from: from,
	}
}

func (p *SMTPProvider) Name() string { return "smtp" }

func (p *SMTPProvider) Supports(ch Channel) bool { return ch == ChannelEmail }

func (p *SMTPProvider) Send(_ context.Context, msg Message) error {
	// 剥离 \r\n 防止 SMTP 头注入（防御纵深，上游已做格式校验）
	target := sanitizeHeader(msg.Target)
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: 验证码\r\n\r\n您的验证码是 %s，5 分钟内有效。\r\n",
		p.from, target, msg.Code)
	return smtp.SendMail(p.addr, p.auth, p.from, []string{target}, []byte(body))
}

// sanitizeHeader 剥离字符串中的 CR/LF，防止注入额外邮件头。
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}
