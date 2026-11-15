package sender

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// startFakeSMTP 启动一个最小 SMTP 服务端：可控制是否声明 STARTTLS、是否只接受连接不响应。
func startFakeSMTP(t *testing.T, advertiseSTARTTLS, silent bool) (string, int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()

		if silent {
			// 只接受连接、永不响应：模拟黑洞 SMTP，用于验证超时/取消能打断阻塞读。
			_, _ = bufio.NewReader(conn).ReadString('\n')
			return
		}

		fmt.Fprint(conn, "220 fake ESMTP\r\n")
		reader := bufio.NewReader(conn)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				if advertiseSTARTTLS {
					fmt.Fprint(conn, "250-fake\r\n250-STARTTLS\r\n250 HELP\r\n")
				} else {
					fmt.Fprint(conn, "250-fake\r\n250 HELP\r\n")
				}
			case strings.HasPrefix(cmd, "DATA"):
				fmt.Fprint(conn, "354 End data with <CR><LF>.<CR><LF>\r\n")
				for {
					dataLine, dataErr := reader.ReadString('\n')
					if dataErr != nil {
						return
					}
					if strings.TrimRight(dataLine, "\r\n") == "." {
						break
					}
				}
				fmt.Fprint(conn, "250 OK\r\n")
			case strings.HasPrefix(cmd, "QUIT"):
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "250 OK\r\n")
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

// TestSMTPProvider_RequiresSTARTTLSForAuth 固定「配置了凭据就不允许明文提交」这条契约。
func TestSMTPProvider_RequiresSTARTTLSForAuth(t *testing.T) {
	host, port := startFakeSMTP(t, false, false)
	p := NewSMTPProvider(host, port, "user", "secret", "no-reply@example.com")

	err := p.Send(context.Background(), Message{Channel: ChannelEmail, Target: "a@b.c", Code: "123456"})
	if err == nil {
		t.Fatal("expected error when server does not advertise STARTTLS and credentials are configured")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("error should mention STARTTLS, got %v", err)
	}
}

// TestSMTPProvider_NoAuthAllowsPlaintextRelay 验证无凭据的内网中继仍可用（没有密码可泄漏）。
func TestSMTPProvider_NoAuthAllowsPlaintextRelay(t *testing.T) {
	host, port := startFakeSMTP(t, false, false)
	// 服务端未实现 MAIL/RCPT/DATA 的完整流程，但会回 250，足以证明客户端不会在
	// 「未声明 STARTTLS」这一步就拒绝无凭据连接。
	p := NewSMTPProvider(host, port, "", "", "no-reply@example.com")
	p.sendTimeout = 2 * time.Second

	if err := p.Send(context.Background(), Message{Channel: ChannelEmail, Target: "a@b.c", Code: "123456"}); err != nil {
		t.Fatalf("unauthenticated relay should not require STARTTLS, got %v", err)
	}
}

// TestSMTPProvider_ContextCancellationInterruptsBlockingIO 验证 ctx 到期能立刻打断黑洞连接。
func TestSMTPProvider_ContextCancellationInterruptsBlockingIO(t *testing.T) {
	host, port := startFakeSMTP(t, false, true)
	p := NewSMTPProvider(host, port, "", "", "no-reply@example.com")
	p.sendTimeout = 30 * time.Second // 远大于 ctx 超时，确保打断来自 ctx 而不是 sendTimeout

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := p.Send(ctx, Message{Channel: ChannelEmail, Target: "a@b.c", Code: "123456"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error for black-holed SMTP server")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("send blocked for %s; ctx cancellation did not interrupt I/O", elapsed)
	}
}
