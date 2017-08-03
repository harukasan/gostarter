# gostarter

[![Build Status](https://travis-ci.org/harukasan/gostarter.svg)](https://travis-ci.org/harukasan/gostarter)
[![GoDoc](https://godoc.org/github.com/harukasan/gostarter?status.svg)](https://godoc.org/github.com/harukasan/gostarter)

`gostarter` is a utility to implement hot-deploying server using
[start_server](http://search.cpan.org/~kazuho/Server-Starter-0.11/start_server).

It include:

 - monitoring worker using HTTP health or custom monitor
 - generating single file `start_server` using [gen_start_server.sh](./gen_start_server.sh)

To see full feature: run `go run example/http/main.go -mode=master`.
