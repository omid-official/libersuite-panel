package sshserver

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/libersuite-org/panel/database"
	"github.com/libersuite-org/panel/database/models"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	Host    string
	Port    int
	HostKey string
}

type Server struct {
	cfg         *Config
	server      *ssh.Server
	sessions    map[string]*sessionTracker
	connections map[string]*gossh.ServerConn
	mu          sync.RWMutex
	wg          sync.WaitGroup
	ctx         context.Context
}

type sessionTracker struct {
	client       *models.Client
	bytesRead    int64
	bytesWritten int64
	startTime    time.Time
	conns        sync.Map
}

func New(cfg *Config) *Server {
	return &Server{
		cfg:         cfg,
		sessions:    make(map[string]*sessionTracker),
		connections: make(map[string]*gossh.ServerConn),
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.ctx = ctx

	server := &ssh.Server{
		Addr:            fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port),
		PasswordHandler: s.passwordHandler,
		LocalPortForwardingCallback: func(ctx ssh.Context, dhost string, dport uint32) bool {
			log.Printf("Local port forwarding request from %s to %s:%d", ctx.User(), dhost, dport)
			return true
		},
		ReversePortForwardingCallback: func(ctx ssh.Context, bindHost string, bindPort uint32) bool {
			return false
		},
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": s.directTCPIPHandler,
		},
	}

	if s.cfg.HostKey != "" {
		if err := server.SetOption(ssh.HostKeyFile(s.cfg.HostKey)); err != nil {
			log.Printf("Warning: Failed to set host key: %v", err)
		}
	}

	s.server = server
	log.Printf("Starting SSH server on %s:%d", s.cfg.Host, s.cfg.Port)

	s.wg.Add(1)
	go s.usageFlusher()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Println("Context cancelled, initiating shutdown...")
		return nil
	case err := <-errChan:
		return err
	}
}

func (s *Server) usageFlusher() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flushAll()
		case <-s.ctx.Done():
			s.flushAll()
			return
		}
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Starting graceful shutdown...")

	if s.server != nil {
		if err := s.server.Close(); err != nil {
			log.Printf("Error closing SSH server: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		log.Println("Shutdown timeout reached, forcing exit")
	}

	s.flushAll()
	return nil
}

func (s *Server) passwordHandler(ctx ssh.Context, password string) bool {
	username := ctx.User()

	var client models.Client
	if err := database.DB.Where("username = ?", username).First(&client).Error; err != nil {
		log.Printf("Authentication failed for user '%s': user not found", username)
		return false
	}

	if client.Password != password {
		log.Printf("Authentication failed for user '%s': invalid password", username)
		return false
	}

	if !client.IsActive() {
		log.Printf("Authentication failed for user '%s': account inactive", username)
		return false
	}

	client.LastConnection = time.Now()
	database.DB.Save(&client)

	ctx.SetValue("client", &client)

	log.Printf("User '%s' authenticated successfully", username)
	return true
}

func (s *Server) directTCPIPHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	clientInterface := ctx.Value("client")
	if clientInterface == nil {
		newChan.Reject(gossh.Prohibited, "authentication required")
		return
	}

	client := clientInterface.(*models.Client)
	sessionID := ctx.SessionID()

	tracker := s.getOrCreateSession(sessionID, client, conn)

	var drtMsg struct {
		DestAddr string
		DestPort uint32
		OrigAddr string
		OrigPort uint32
	}

	if err := gossh.Unmarshal(newChan.ExtraData(), &drtMsg); err != nil {
		newChan.Reject(gossh.ConnectionFailed, "invalid direct-tcpip request")
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		return
	}
	defer ch.Close()

	go gossh.DiscardRequests(reqs)

	dest := fmt.Sprintf("%s:%d", drtMsg.DestAddr, drtMsg.DestPort)

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	dconn, err := dialer.DialContext(s.ctx, "tcp", dest)
	if err != nil {
		log.Printf("Failed to connect to %s: %v", dest, err)
		return
	}
	defer dconn.Close()

	tracker.conns.Store(dconn, struct{}{})
	defer tracker.conns.Delete(dconn)

	tracker.conns.Store(ch, struct{}{})
	defer tracker.conns.Delete(ch)

	s.wg.Add(1)
	defer s.wg.Done()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		tr := &trafficReader{reader: ch, tracker: tracker, client: client}
		_, _ = io.Copy(dconn, tr)
	}()

	go func() {
		defer wg.Done()
		tw := &trafficWriter{writer: ch, tracker: tracker, client: client}
		_, _ = io.Copy(tw, dconn)
	}()

	wg.Wait()
}

func (s *Server) getOrCreateSession(id string, client *models.Client, conn *gossh.ServerConn) *sessionTracker {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t, ok := s.sessions[id]; ok {
		return t
	}

	t := &sessionTracker{
		client:    client,
		startTime: time.Now(),
	}
	s.sessions[id] = t
	s.connections[id] = conn

	s.wg.Add(1)
	go s.watchSession(id, conn)

	return t
}

func (s *Server) watchSession(id string, conn *gossh.ServerConn) {
	defer s.wg.Done()
	conn.Wait()

	s.mu.Lock()
	tracker := s.sessions[id]
	delete(s.sessions, id)
	delete(s.connections, id)
	s.mu.Unlock()

	if tracker != nil {
		tracker.conns.Range(func(key, _ any) bool {
			switch c := key.(type) {
			case net.Conn:
				_ = c.Close()
			case io.Closer:
				_ = c.Close()
			}
			return true
		})

		s.flushOne(tracker)
		log.Printf("Session %s closed (%s)", id, tracker.client.Username)
	}
}

func (s *Server) flushAll() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, t := range s.sessions {
		s.flushOne(t)
	}
}

func (s *Server) flushOne(t *sessionTracker) {
	used := atomic.SwapInt64(&t.bytesRead, 0) + atomic.SwapInt64(&t.bytesWritten, 0)
	if used == 0 {
		return
	}

	t.client.TrafficUsed += used
	database.DB.Save(t.client)
}

type trafficReader struct {
	reader  io.Reader
	tracker *sessionTracker
	client  *models.Client
}

func (tr *trafficReader) Read(p []byte) (n int, err error) {
	n, err = tr.reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&tr.tracker.bytesRead, int64(n))

		if tr.client.TrafficLimit > 0 {
			totalUsed := tr.client.TrafficUsed + atomic.LoadInt64(&tr.tracker.bytesRead) + atomic.LoadInt64(&tr.tracker.bytesWritten)
			if totalUsed >= tr.client.TrafficLimit {
				return n, io.EOF
			}
		}
	}
	return n, err
}

type trafficWriter struct {
	writer  io.Writer
	tracker *sessionTracker
	client  *models.Client
}

func (tw *trafficWriter) Write(p []byte) (n int, err error) {
	n, err = tw.writer.Write(p)
	if n > 0 {
		atomic.AddInt64(&tw.tracker.bytesWritten, int64(n))

		if tw.client.TrafficLimit > 0 {
			totalUsed := tw.client.TrafficUsed + atomic.LoadInt64(&tw.tracker.bytesRead) + atomic.LoadInt64(&tw.tracker.bytesWritten)
			if totalUsed >= tw.client.TrafficLimit {
				return n, io.ErrShortWrite
			}
		}
	}
	return n, err
}
