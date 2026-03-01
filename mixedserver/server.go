package mixedserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const socksVersion5 = 0x05

type Config struct {
	Host        string
	Port        int
	BackendHost string
	SSHPort     int
	SOCKSPort   int
}

type Server struct {
	cfg      *Config
	listener net.Listener
	ctx      context.Context
	wg       sync.WaitGroup
}

func New(cfg *Config) *Server {
	return &Server{cfg: cfg}
}

func (s *Server) Start(ctx context.Context) error {
	s.ctx = ctx
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start mixed listener on %s: %w", addr, err)
	}

	s.listener = listener
	log.Printf("Starting mixed SSH/SOCKS listener on %s", addr)

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
			log.Printf("Mixed accept error: %v", err)
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

func (s *Server) handleConnection(clientConn net.Conn) {
	defer s.wg.Done()
	defer clientConn.Close()

	buffer := make([]byte, 1)
	hasFirstByte := false

	_ = clientConn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	n, err := clientConn.Read(buffer)
	if err == nil && n == 1 {
		hasFirstByte = true
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		if err != io.EOF {
			log.Printf("Mixed read probe error: %v", err)
		}
		return
	}
	_ = clientConn.SetReadDeadline(time.Time{})

	targetPort := s.cfg.SSHPort
	if hasFirstByte && buffer[0] == socksVersion5 {
		targetPort = s.cfg.SOCKSPort
	}

	targetAddr := fmt.Sprintf("%s:%d", s.cfg.BackendHost, targetPort)
	targetConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		log.Printf("Mixed dial backend %s failed: %v", targetAddr, err)
		return
	}
	defer targetConn.Close()

	if hasFirstByte {
		if _, err := targetConn.Write(buffer); err != nil {
			log.Printf("Mixed forward first byte to %s failed: %v", targetAddr, err)
			return
		}
	}

	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = clientConn.Close()
			_ = targetConn.Close()
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(targetConn, clientConn)
		closeBoth()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, targetConn)
		closeBoth()
	}()

	wg.Wait()
}
