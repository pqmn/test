package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// Engine 自定义web服务引擎
type Engine struct {
	// 路由模式及其响应函数映射表
	handle map[string]func(w http.ResponseWriter, r *http.Request)
}

// NewEngine 构造引擎
func NewEngine() *Engine {
	handle := make(map[string]func(w http.ResponseWriter, r *http.Request))
	return &Engine{
		handle: handle,
	}
}

// Api 为当前服务引擎添加接口
func (e *Engine) Api(pattern string, handle func(w http.ResponseWriter, r *http.Request)) {
	if pattern == "" {
		panic("must pattern")
	}
	// 模式不存在，则添加，反之报错
	_, ok := e.handle[pattern]
	if !ok {
		e.handle[pattern] = handle
	} else {
		panic("duplicate pattern")
	}
}

// 实现Handler接口，使监听服务时可传入自定义引擎，执行自定义方法
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 根据路径模式获取响应函数并执行
	path := r.URL.Path
	handle, ok := e.handle[path]
	if !ok {
		panic("not found")
	}
	handle(w, r)
}

func main() {
	go ListenPortHttp()  // 启动http服务
	go ListenPortHttps() // 启动https服务

	// 监听9922端口
	listener, err := net.Listen("tcp", "10.18.13.101:9222")
	if err != nil {
		fmt.Println("tcp listen err:", err)
		return
	}
	defer listener.Close()

	// 循环获取数据，并判断此数据是http服务还是https服务，再将数据转发到相应服务的监听端口
	conn, err := listener.Accept()
	if err != nil {
		fmt.Println("accept err:", err)
		return
	}
	for {
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		// 若一开始获取到连接reset的消息，则直接跳过
		if string(buf[:n]) == "connection-reset" {
			continue
		}

		var addr string
		if bytes.HasPrefix(buf, []byte("GET")) {
			addr = "localhost:8855"
		} else {
			addr = "localhost:8854"
		}

		// 获取服务端连接
		server, err := net.Dial("tcp", addr)
		if err != nil {
			continue
		}
		// 将从A-B间tcp连接中读取的请求数据写入当前服务端连接
		n, err = server.Write(buf[:n])
		if err != nil {
			server.Close()
			continue
		}

		// 保持当前连接，继续传输数据
		go func() {
			// 将从A-B间tcp连接中读取的请求数据写入当前服务端连接
			buf2 := make([]byte, 4096)
			for {
				err := server.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
				if err != nil {
					return
				}

				n, err = conn.Read(buf2)
				if err != nil {
					break
				}
				if string(buf2[:n]) == "connection-reset" {
					// 收到连接reset消息，退出当前服务端连接，进入下一循环获取新的服务端连接
					break
				}
				n, err = server.Write(buf2[:n])
				if err != nil {
					break
				}
			}
			server.Close()
		}()

		// 将当前服务端连接中的响应数据写入A-B间tcp连接
		io.Copy(conn, server)
		server.Close()
	}
}

// ListenPortHttps 建立B设备上的https服务
func ListenPortHttps() {
	engine := NewEngine()
	engine.Api("/", handleHttpsGetMac)
	log.Fatal(http.ListenAndServeTLS("localhost:8854", "./cert.pem", "./key.pem", engine))
}

// ListenPortHttp 建立B设备上的http服务
func ListenPortHttp() {
	engine := NewEngine()
	engine.Api("/", handleHttpGetMac)
	log.Fatal(http.ListenAndServe("localhost:8855", engine))
}

// handleHttpGetMac 处理http获取mac的请求
func handleHttpGetMac(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte(fmt.Sprintf("http B mac 地址是：%s", getMac())))
	if err != nil {
		panic(err)
		return
	}
}

// handleHttpsGetMac 处理https获取mac的请求
func handleHttpsGetMac(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte(fmt.Sprintf("https B mac 地址是：%s", getMac())))
	if err != nil {
		panic(err)
		return
	}
}

func getMac() string {
	inter, err := net.InterfaceByName("eth0")
	if err != nil {
		return err.Error()
	}
	return inter.HardwareAddr.String()
}

