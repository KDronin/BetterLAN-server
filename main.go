package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
)

type Config struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type Session struct {
	guestConn net.Conn
}

type Group struct {
	password        string
	hosts           map[string]net.Conn
	pendingSessions map[string]Session
	mu              sync.Mutex
}

var (
	groups   = make(map[string]*Group)
	globalMu sync.Mutex
)

func generateSessionID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func main() {
	configFile, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println("错误: 未找到 config.json。请先使用官方一键安装脚本进行部署！")
		os.Exit(1)
	}

	var config Config
	if err := json.Unmarshal(configFile, &config); err != nil {
		fmt.Println("错误: config.json 格式损坏，解析失败:", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%d", config.IP, config.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("启动服务失败，端口 %d 可能被占用: %v\n", config.Port, err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("BetterLAN server节点已启动，当前绑定 %s\n", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	reader := bufio.NewReader(conn)
	
	line, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return
	}

	msg := strings.TrimSpace(line)
	parts := strings.Split(msg, "|")
	if len(parts) == 0 {
		conn.Close()
		return
	}

	action := parts[0]

	switch action {
	case "LIST":
		if len(parts) >= 3 {
			grp, pwd := parts[1], parts[2]
			globalMu.Lock()
			group, exists := groups[grp]
			globalMu.Unlock()

			if exists && group.password == pwd {
				group.mu.Lock()
				var hostNames []string
				for name := range group.hosts {
					hostNames = append(hostNames, name)
				}
				group.mu.Unlock()
				conn.Write([]byte(strings.Join(hostNames, ",") + "\n"))
			} else {
				conn.Write([]byte("ERROR|Auth Failed\n"))
			}
		}
		conn.Close()

	case "HOST_LISTEN":
		if len(parts) >= 4 {
			grp, pwd, name := parts[1], parts[2], parts[3]
			
			globalMu.Lock()
			group, exists := groups[grp]
			if !exists {
				group = &Group{
					password:        pwd,
					hosts:           make(map[string]net.Conn),
					pendingSessions: make(map[string]Session),
				}
				groups[grp] = group
			}
			globalMu.Unlock()

			if group.password != pwd {
				conn.Write([]byte("ERROR|Wrong Password\n"))
				conn.Close()
				return
			}

			group.mu.Lock()
			group.hosts[name] = conn
			group.mu.Unlock()
			fmt.Printf("[+] 房主已注册: 组[%s] -> 玩家[%s]\n", grp, name)

			buf := make([]byte, 10)
			conn.Read(buf) 

			group.mu.Lock()
			delete(group.hosts, name)
			group.mu.Unlock()
			fmt.Printf("[-] 房主已断开: 组[%s] -> 玩家[%s]\n", grp, name)
		}
		conn.Close()

	case "GUEST_JOIN":
		if len(parts) >= 4 {
			grp, pwd, target := parts[1], parts[2], parts[3]
			
			globalMu.Lock()
			group, exists := groups[grp]
			globalMu.Unlock()

			if exists && group.password == pwd {
				group.mu.Lock()
				hostConn, hostExists := group.hosts[target]
				if hostExists {
					sessionID := generateSessionID()
					group.pendingSessions[sessionID] = Session{guestConn: conn}
					
					hostConn.Write([]byte(fmt.Sprintf("INCOMING|%s\n", sessionID)))
					fmt.Printf("[*] 通知房主 [%s] 对接会话: %s\n", target, sessionID)
					group.mu.Unlock()
					return 
				}
				group.mu.Unlock()
			}
			conn.Write([]byte("ERROR|Auth or Host Not Found\n"))
		}
		conn.Close()

	case "HOST_ACCEPT":
		if len(parts) >= 4 {
			grp, pwd, sessionID := parts[1], parts[2], parts[3]
			
			globalMu.Lock()
			group, exists := groups[grp]
			globalMu.Unlock()

			if exists && group.password == pwd {
				group.mu.Lock()
				session, sessionExists := group.pendingSessions[sessionID]
				if sessionExists {
					delete(group.pendingSessions, sessionID)
					group.mu.Unlock()

					guestConn := session.guestConn
					guestConn.Write([]byte("OK\n"))
					fmt.Printf("[!] 隧道双向对接成功: %s\n", sessionID)

					go pipe(conn, guestConn)
					go pipe(guestConn, conn)
					return
				}
				group.mu.Unlock()
			}
		}
		conn.Close()

	default:
		conn.Close()
	}
}

func pipe(src net.Conn, dst net.Conn) {
	io.Copy(dst, src)
	dst.Close()
	src.Close()
}
