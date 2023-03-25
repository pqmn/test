package main

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

func main() {
	// 与B设备建立tcp连接
	netAddr := &net.TCPAddr{Port: 9222}
	d := net.Dialer{LocalAddr: netAddr}
	server, err := d.Dial("tcp", "10.18.13.101:9222")
	if err != nil {
		fmt.Println("dial err:", err)
		return
	}

	wg := new(sync.WaitGroup)
	// 监听本地端口8855，用于转发http请求
	listen1, err := net.Listen("tcp", ":8855")
	if err != nil {
		fmt.Println("listen 8855 err:", err)
		return
	}
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		for {
			conn, err := listen1.Accept()
			if err != nil {
				fmt.Println("accept 8855 err:", err)
				return
			}
			handleConn(conn, server)
		}
		wg.Done()
	}(wg)

	// 监听本地端口8854，用于转发https请求
	listen2, err := net.Listen("tcp", ":8854")
	if err != nil {
		fmt.Println("listen 8854 err:", err)
		return
	}
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		for {
			conn, err := listen2.Accept()
			if err != nil {
				fmt.Println("accept 8854 err:", err)
				conn.Close()
				return
			}
			// 发送http请求后发送https连接reset，告知服务端重新获取服务连接
			server.Write([]byte("connection-reset"))
			handleConn(conn, server)
		}
		wg.Done()
	}(wg)

	wg.Wait()
}

func handleConn(conn, server net.Conn) {
	go func() {
		defer conn.Close()
		// 将当前客户端连接中的请求数据写入A-B间tcp连接
		_, err := io.Copy(server, conn)
		if err != nil {
			// https请求后连接reset，告知服务端重新获取服务连接
			if strings.Contains(err.Error(), "splice: connection reset by peer") {
				server.Write([]byte("connection-reset"))
			}
		}
	}()

	go func() {
		defer conn.Close()
		// 将从A-B间tcp连接中读取的响应数据写入当前客户端连接
		buf := make([]byte, 4096)
		for {
			err := server.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
			if err != nil {
				return
			}
			n, err := server.Read(buf)
			if err != nil {
				return
			}

			n, err = conn.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
}

