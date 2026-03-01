package socksserver

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libersuite-org/panel/database"
	"github.com/libersuite-org/panel/database/models"
	"gorm.io/gorm"
)

const (
	socksVersion5       = 0x05
	authMethodUserPass  = 0x02
	authMethodNoAccept  = 0xFF
	userPassVersion     = 0x01
	socksCmdConnect     = 0x01
	addrTypeIPv4        = 0x01
	addrTypeDomain      = 0x03
	addrTypeIPv6        = 0x04
	replySucceeded      = 0x00
	replyGeneralFailure = 0x01
	replyCmdNotSupport  = 0x07
	replyAddrNotSupport = 0x08
)

type Config struct {
	Host string
	Port int
}

type Server struct {
	cfg      *Config
	listener net.Listener
	ctx      context.Context
	wg       sync.WaitGroup
}

type quotaWriter struct {
	writer   io.Writer
	used     *int64
	baseUsed int64
	limit    int64
}

func (q *quotaWriter) Write(p []byte) (n int, err error) {
	n, err = q.writer.Write(p)
	if n > 0 {
		total := atomic.AddInt64(q.used, int64(n)) + q.baseUsed
		if q.limit > 0 && total >= q.limit {
			return n, io.ErrShortWrite
		}
	}
	return n, err
}

func New(cfg *Config) *Server {
	return &Server{cfg: cfg}
}

func (s *Server) Start(ctx context.Context) error {
	s.ctx = ctx
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start SOCKS listener on %s: %w", addr, err)
	}

	s.listener = listener
	log.Printf("Starting SOCKS5 server on %s", addr)

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			log.Printf("SOCKS accept error: %v", err)
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.listener != nil {
		_ = s.listener.Close()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	client, err := authenticate(conn)
	if err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})

	if err := s.handleConnectRequest(conn, client); err != nil {
		log.Printf("SOCKS request failed for user '%s': %v", client.Username, err)
	}
}

func authenticate(conn net.Conn) (*models.Client, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	if header[0] != socksVersion5 {
		return nil, fmt.Errorf("unsupported SOCKS version: %d", header[0])
	}

	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return nil, err
	}

	if !hasMethod(methods, authMethodUserPass) {
		_, _ = conn.Write([]byte{socksVersion5, authMethodNoAccept})
		return nil, errors.New("client does not support username/password auth")
	}

	if _, err := conn.Write([]byte{socksVersion5, authMethodUserPass}); err != nil {
		return nil, err
	}

	upHeader := make([]byte, 2)
	if _, err := io.ReadFull(conn, upHeader); err != nil {
		return nil, err
	}

	if upHeader[0] != userPassVersion {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return nil, errors.New("invalid auth version")
	}

	userLen := int(upHeader[1])
	if userLen == 0 {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return nil, errors.New("empty username")
	}

	username := make([]byte, userLen)
	if _, err := io.ReadFull(conn, username); err != nil {
		return nil, err
	}

	passLenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, passLenBuf); err != nil {
		return nil, err
	}

	passLen := int(passLenBuf[0])
	password := make([]byte, passLen)
	if _, err := io.ReadFull(conn, password); err != nil {
		return nil, err
	}

	var client models.Client
	if err := database.DB.Where("username = ?", string(username)).First(&client).Error; err != nil {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return nil, errors.New("invalid username or password")
	}

	if client.Password != string(password) || !client.IsActive() {
		_, _ = conn.Write([]byte{userPassVersion, 0x01})
		return nil, errors.New("invalid username or password")
	}

	client.LastConnection = time.Now()
	_ = database.DB.Save(&client).Error

	if _, err := conn.Write([]byte{userPassVersion, 0x00}); err != nil {
		return nil, err
	}

	log.Printf("SOCKS user '%s' authenticated", client.Username)
	return &client, nil
}

func hasMethod(methods []byte, method byte) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func (s *Server) handleConnectRequest(conn net.Conn, client *models.Client) error {
	requestHeader := make([]byte, 4)
	if _, err := io.ReadFull(conn, requestHeader); err != nil {
		return err
	}

	if requestHeader[0] != socksVersion5 {
		return errors.New("invalid SOCKS request version")
	}

	if requestHeader[1] != socksCmdConnect {
		_ = writeReply(conn, replyCmdNotSupport)
		return errors.New("unsupported SOCKS command")
	}

	address, err := readTargetAddress(conn, requestHeader[3])
	if err != nil {
		_ = writeReply(conn, replyAddrNotSupport)
		return err
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	targetConn, err := dialer.DialContext(s.ctx, "tcp", address)
	if err != nil {
		_ = writeReply(conn, replyGeneralFailure)
		return fmt.Errorf("failed to connect to %s: %w", address, err)
	}
	defer targetConn.Close()

	if err := writeReply(conn, replySucceeded); err != nil {
		return err
	}

	var sessionUsed int64
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = conn.Close()
			_ = targetConn.Close()
		})
	}

	upstream := &quotaWriter{
		writer:   targetConn,
		used:     &sessionUsed,
		baseUsed: client.TrafficUsed,
		limit:    client.TrafficLimit,
	}

	downstream := &quotaWriter{
		writer:   conn,
		used:     &sessionUsed,
		baseUsed: client.TrafficUsed,
		limit:    client.TrafficLimit,
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, conn)
		closeBoth()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(downstream, targetConn)
		closeBoth()
	}()

	wg.Wait()

	used := atomic.LoadInt64(&sessionUsed)
	if used > 0 {
		if err := database.DB.Model(&models.Client{}).
			Where("id = ?", client.ID).
			UpdateColumn("traffic_used", gorm.Expr("traffic_used + ?", used)).Error; err != nil {
			log.Printf("Failed to update traffic usage for SOCKS user '%s': %v", client.Username, err)
		}
	}

	return nil
}

func readTargetAddress(conn net.Conn, atyp byte) (string, error) {
	var host string

	switch atyp {
	case addrTypeIPv4:
		ip := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()
	case addrTypeIPv6:
		ip := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return "", err
		}
		host = net.IP(ip).String()
	case addrTypeDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", err
		}

		domainLen := int(lenBuf[0])
		if domainLen == 0 {
			return "", errors.New("invalid domain length")
		}

		domain := make([]byte, domainLen)
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", err
		}
		host = string(domain)
	default:
		return "", errors.New("unsupported address type")
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", err
	}

	port := binary.BigEndian.Uint16(portBuf)
	return fmt.Sprintf("%s:%d", host, port), nil
}

func writeReply(conn net.Conn, rep byte) error {
	reply := []byte{socksVersion5, rep, 0x00, addrTypeIPv4, 0, 0, 0, 0, 0, 0}
	_, err := conn.Write(reply)
	return err
}
