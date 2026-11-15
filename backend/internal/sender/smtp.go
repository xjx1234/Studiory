package sender

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSMTPDialTimeout = 5 * time.Second
	defaultSMTPSendTimeout = 10 * time.Second
)

// SMTPProvider 通过 SMTP 发送邮件验证码，是一个可直接使用的真实参考实现。
//
// 与 net/smtp.SendMail 相比，这里显式控制三件事，避免一次慢/黑洞 SMTP 连接把
// /api/v1/auth/send-code 的 goroutine 长期挂住：
//   - 建连超时（DialContext + dialTimeout）；
//   - 全流程 deadline（sendTimeout，且被 ctx 的更早 deadline 收敛）；
//   - ctx 取消立即中断阻塞中的读写（context.AfterFunc 把连接 deadline 设为当前时刻）。
//
// 配置了凭据（username 非空）时强制要求 STARTTLS：net/smtp 在服务端未声明 STARTTLS
// 时会明文提交认证信息，那等于把邮箱密码交给链路上的任何人。
type SMTPProvider struct {
	host string // 用于 EHLO 与 TLS ServerName
	addr string // host:port
	auth smtp.Auth
	from string

	dialTimeout time.Duration
	sendTimeout time.Duration
}

// NewSMTPProvider 构造 SMTP 邮件 Provider。username 为空时不做鉴权（适合内网中继）。
func NewSMTPProvider(host string, port int, username, password, from string) *SMTPProvider {
	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}
	return &SMTPProvider{
		host:        host,
		addr:        net.JoinHostPort(host, strconv.Itoa(port)),
		auth:        auth,
		from:        from,
		dialTimeout: defaultSMTPDialTimeout,
		sendTimeout: defaultSMTPSendTimeout,
	}
}

func (p *SMTPProvider) Name() string { return "smtp" }

func (p *SMTPProvider) Supports(ch Channel) bool { return ch == ChannelEmail }

func (p *SMTPProvider) Send(ctx context.Context, msg Message) error {
	// 剥离 \r\n 防止 SMTP 头注入（防御纵深，上游已做格式校验）
	target := sanitizeHeader(msg.Target)
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: 验证码\r\n\r\n您的验证码是 %s，5 分钟内有效。\r\n",
		p.from, target, msg.Code)
	return p.deliver(ctx, target, []byte(body))
}

func (p *SMTPProvider) deliver(ctx context.Context, to string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("smtp send canceled: %w", err)
	}

	dialer := &net.Dialer{Timeout: p.dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", p.addr, err)
	}
	defer func() { _ = conn.Close() }()

	// ctx 取消时立刻把连接 deadline 拉到当前时刻，让阻塞中的 SMTP 读写返回错误，
	// 而不是等 sendTimeout 到点。AfterFunc 返回的 stop 必须在退出前调用。
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stop()

	deadline := time.Now().Add(p.sendTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("smtp set deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, p.host)
	if err != nil {
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer func() { _ = client.Close() }()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{
			ServerName: p.host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	} else if p.auth != nil {
		return errors.New("smtp: server does not advertise STARTTLS; refusing to send credentials in plaintext")
	}

	if p.auth != nil {
		if err := client.Auth(p.auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(p.from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := writer.Write(body); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp close body: %w", err)
	}
	return client.Quit()
}

// sanitizeHeader 剥离字符串中的 CR/LF，防止注入额外邮件头。
func sanitizeHeader(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}
