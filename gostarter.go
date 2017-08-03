// Copyright 2017 Shunsuke Michii. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package gostarter provides utilities to run any servers with start_server
process for graceful restarting and hot deployment.

The start_server utility is a superdaemon for hot-deploying server programs.
The package provides two modes:

	* MasterMode:  spawning start_server program for master process.
	* WorkerMode:  receiving sockets from the start_server and run servers.
	               If the worker process is not spawned from the start_server,
								 the worker process will act as a single process server.
	* MonitorMode: running monitor using http or custom health checker.

To run HTTP server:

	s := &http.Server{
		Handler: h,
	}
	gostarter.AddServer(":8000", s)
	return gostarter.Listen(mode)
*/
package gostarter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/lestrrat/go-server-starter/listener"
)

// StartServerProgram specifies the start_server program name.
var StartServerProgram = "start_server"

type Server struct {
	net  string
	addr string

	server   TCPServer
	monitors []Monitor
}

type Monitor interface {
	Run(ctx context.Context, net, addr string, trigger func(error) error) error
}

func (s *Server) AddMonitor(m Monitor) {
	for _, em := range s.monitors {
		if em == m {
			return
		}
	}
	s.monitors = append(s.monitors, m)
}

type Starter struct {
	Interval        time.Duration
	ShutdownTimeout time.Duration
	PidFile         string
	StatusFile      string

	servers []*Server
	err     error
}

type TCPServer interface {
	ServeTCP(net.Listener) error
}

type GracefulServer interface {
	Shutdown(ctx context.Context) error
}

func (s *Starter) AddTCPServer(net, addr string, ts TCPServer) *Server {
	sv := &Server{
		net:    net,
		addr:   addr,
		server: ts,
	}
	s.servers = append(s.servers, sv)
	return sv
}

func (s *Starter) AddServer(net, addr string, server *http.Server) *Server {
	return s.AddTCPServer(net, addr, &httpServer{server})
}

type httpServer struct {
	*http.Server
}

func (s *httpServer) ServeTCP(l net.Listener) error {
	return s.Server.Serve(l)
}

const (
	MasterMode  = "master"
	MonitorMode = "monitor"
	WorkerMode  = "worker"
)

func (s *Starter) Listen(mode string) error {
	if mode == "" {
		mode = os.Getenv("GOSTARTER_MODE")
	}

	if s.err != nil {
		return s.err
	}
	switch mode {
	case MonitorMode:
		return s.startMonitor()
	case MasterMode:
		return s.startMaster()
	case WorkerMode, "":
		return s.startWorker()
	}
	return fmt.Errorf("unknown mode: %v", mode)
}

func (s *Starter) startMonitor() error {
	pid, err := strconv.Atoi(os.Getenv("GOSTARTER_MASTER_PID"))
	if err != nil {
		return fmt.Errorf("failed to parse GOSTARTER_MASTER_PID: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, server := range s.servers {
		for _, monitor := range server.monitors {
			fmt.Fprintf(os.Stderr, "starting monitor to %s:%s\n", server.net, server.addr)
			go func(m Monitor) {
				m.Run(ctx, server.net, server.addr, func(err error) error {
					fmt.Fprintf(os.Stderr, "monitor: sending SIGHUP to: %d, because: %v\n", pid, err)
					return syscall.Kill(pid, syscall.SIGHUP)
				})
				cancel()
			}(monitor)
		}
	}
	select {
	case <-ctx.Done():
		err := ctx.Err()
		if err != context.Canceled {
			return err
		}
		return nil
	}
}

func spawnMonitor() error {
	if listener.GetPortsSpecification() == "" {
		return nil
	}
	ppid := syscall.Getppid()
	if ppid <= 0 {
		return nil
	}

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, "GOSTARTER_MODE=monitor")
	cmd.Env = append(cmd.Env, fmt.Sprintf("GOSTARTER_MASTER_PID=%d", ppid))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Start()
}

func (s *Starter) startWorker() error {

	ls, err := listener.ListenAll()
	if err == listener.ErrNoListeningTarget {
		ls, err = s.listenAll()
	}
	if err != nil {
		return err
	}
	if len(ls) == 0 {
		return errors.New("missing listeners")
	}

	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGQUIT)
	signal.Ignore(syscall.SIGPIPE)

	// start all servers
	errCh := make(chan error)
	for i, l := range ls {
		go func(srv TCPServer, l net.Listener, errCh chan<- error) {
			errCh <- srv.ServeTCP(l)
		}(s.servers[i].server, l, errCh)
	}

	spawnMonitor()

	select {
	case sig := <-sigCh:
		switch sig {
		case syscall.SIGQUIT:
			return s.shutdown()
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Starter) listenAll() ([]net.Listener, error) {
	ls := make([]net.Listener, len(s.servers))
	var err error
	for i, v := range s.servers {
		ls[i], err = net.Listen(v.net, v.addr)
		if err != nil {
			break
		}
	}
	return ls, err
}

func (s *Starter) shutdown() error {
	ctx := context.Background()

	var cancel func()
	if s.ShutdownTimeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), s.ShutdownTimeout)
		defer cancel()
	}

	retCh := make(chan error)
	for _, v := range s.servers {
		go func(s TCPServer) {
			if sv, ok := s.(GracefulServer); ok {
				retCh <- sv.Shutdown(ctx)
				return
			}
			retCh <- nil
		}(v.server)
	}

	// wait for all shutdown processes
	doneCh := make(chan error)
	go func() {
		for range s.servers {
			if err := <-retCh; err != nil {
				doneCh <- err
				return
			}
		}
		doneCh <- nil
	}()

	select {
	case err := <-doneCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Starter) startMaster() error {
	args, err := s.buildStartServerArgs()
	if err != nil {
		return err
	}
	return syscall.Exec(args[0], args, os.Environ())
}

func (s *Starter) buildStartServerArgs() ([]string, error) {
	interval := int(s.Interval.Seconds())
	if float64(interval) != s.Interval.Seconds() {
		return nil, errors.New("accuracy of the interval must be second")
	}

	args := []string{StartServerProgram}

	for _, v := range s.servers {
		switch v.net {
		case "tcp", "tcp4", "tcp6", "":
			tcp, err := net.ResolveTCPAddr(v.net, v.addr)
			if err != nil {
				return nil, err
			}
			var port string
			if tcp.IP == nil {
				port = fmt.Sprintf("%d", tcp.Port)
			} else {
				port = fmt.Sprintf("%s:%d", tcp.IP, tcp.Port)
			}
			args = append(args, fmt.Sprintf("--port=%s", port))

		case "unix", "unixgram", "unixpacket":
			unix, err := net.ResolveUnixAddr(v.net, v.addr)
			if err != nil {
				return nil, err
			}
			args = append(args, fmt.Sprintf("--path=%s", unix.Name))
		}
	}

	if interval > 0 {
		args = append(args, fmt.Sprintf("--interval=%d", interval))
	}
	if s.PidFile != "" {
		args = append(args, fmt.Sprintf("--pid-file=%s", s.PidFile))
	}
	if s.StatusFile != "" {
		args = append(args, fmt.Sprintf("--status-file=%s", s.StatusFile))
	}
	args = append(args, fmt.Sprintf("--signal-on-hup=%s", "SIGQUIT"))

	selfpath, _ := filepath.Abs(os.Args[0])
	args = append(args, selfpath)

	return args, nil
}

var DefaultStarter = &Starter{}

func AddServer(net, addr string, s *http.Server) *Server {
	return DefaultStarter.AddServer(net, addr, s)
}

func AddTCPServer(net, addr string, s TCPServer) *Server {
	return DefaultStarter.AddTCPServer(net, addr, s)
}

func Listen(mode string) error {
	return DefaultStarter.Listen(mode)
}

func FindStartServerProgram() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		p := path.Join(dir, "start_server")
		_, err := os.Open(p)
		if err != nil && os.IsExist(err) {
			return ""
		}
		if err == nil {
			return p
		}
		dir = path.Dir(dir)
	}
}
