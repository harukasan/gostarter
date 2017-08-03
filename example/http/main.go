// Copyright 2017 Shunsuke Michii. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// example http server
package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/harukasan/gostarter"
	"github.com/harukasan/gostarter/monitor"
)

func main() {
	gostarter.StartServerProgram = gostarter.FindStartServerProgram()

	var addr string
	var mode string
	flag.StringVar(&addr, "addr", ":8080", "`address` to serve HTTP")
	flag.StringVar(&mode, "mode", "", "process mode")

	flag.Parse()

	i := 0
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			i++
			if i > 10 {
				http.Error(w, fmt.Sprintf("n=%d", i), http.StatusInternalServerError)
				return
			}
			http.Error(w, "hello, world!", http.StatusOK)
		}),
	}
	sv := gostarter.AddServer("tcp", addr, server)

	// add a monitor to the server
	sv.AddMonitor(monitor.NewHTTPMonitor(&monitor.HTTPMonitorConfig{
		URL: &url.URL{
			Path: "/",
		},
		ExpectedStatusCode: 200,
		Retry:              2,
	}))

	err := gostarter.Listen(mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}
