// Copyright 2017 Shunsuke Michii. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// example echo server and its client implementation
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"time"

	"github.com/harukasan/gostarter"
)

type echoServer struct{}

func (s *echoServer) ServeTCP(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go func(c net.Conn) {
			io.Copy(c, c)
			c.Close()
		}(conn)
	}
}

func (s *echoServer) Shutdown(ctx context.Context) error {
	time.Sleep(10 * time.Second)
	return nil
}

func send(addr string, msg string) error {
	conn, err := textproto.Dial("tcp", addr)
	if err != nil {
		return err
	}
	conn.Writer.PrintfLine("%s\n", flag.Arg(0))

	str, err := conn.ReadLine()
	fmt.Printf("%s\n", str)
	return err
}

func main() {
	gostarter.StartServerProgram = gostarter.FindStartServerProgram()

	var server string
	var connect string
	var mode string

	flag.StringVar(&server, "server", "", "start echo server with specified `address`")
	flag.StringVar(&connect, "connect", "", "connect to specified echo server")
	flag.StringVar(&mode, "mode", "worker", "server mode")
	flag.Usage = func() {
		fmt.Println("usage:")
		fmt.Printf("  %s -server=ADDRESS\n", os.Args[0])
		fmt.Printf("  %s -connect=ADDRESS [MESSAGE]\n", os.Args[0])
		fmt.Println("\noptions:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if connect != "" {
		err := send(connect, flag.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	gostarter.AddTCPServer("tcp", server, &echoServer{})
	fmt.Fprintf(os.Stderr, "%v\n", gostarter.Listen(mode))
}
