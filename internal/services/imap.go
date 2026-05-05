package services

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

func TestIMAPConnectivity(host string, port int, useTLS bool) error {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	dialer := net.Dialer{Timeout: 8 * time.Second}
	if useTLS {
		conn, err := tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return err
		}
		_ = conn.Close()
		return nil
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
