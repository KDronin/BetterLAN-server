package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type Session struct {
	guestConn net.Conn
}

type GeoInfo struct {
	Zh string
	En string
}

type Presence struct {
	Name       string
	AuthStatus string
	IP         string
	LastSeen   time.Time
}

type Group struct {
	password        string
	hosts           map[string]net.Conn
	pendingSessions map[string]Session
	presences       map[string]*Presence
	mu              sync.RWMutex
}

var (
	groups   = make(map[string]*Group)
	globalMu sync.RWMutex
	geoCache sync.Map
)

func generateSessionID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func getGeoAsync(ipStr string) {
	if _, loaded := geoCache.LoadOrStore(ipStr, &GeoInfo{Zh: "查询中...", En: "Fetching..."}); loaded {
		return
	}

	go func() {
		fetchGeo := func(lang string) string {
			client := http.Client{Timeout: 3 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://ipwho.is/%s?lang=%s", ipStr, lang))
			if err != nil { return "" }
			defer resp.Body.Close()
			var res map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&res)
			
			if success, ok := res["success"].(bool); ok && success {
				country, _ := res["country"].(string)
				region, _ := res["region"].(string)
				if region != "" && region != country {
					return fmt.Sprintf("%s %s", country, region)
				}
				return country
			}
			return ""
		}

		enGeo := fetchGeo("en")
		if enGeo == "" { enGeo = "Unknown Region" }
		zhGeo := fetchGeo("zh-CN")
		if zhGeo == "" { zhGeo = "未知地域" }

		geoCache.Store(ipStr, &GeoInfo{Zh: zhGeo, En: enGeo})
	}()
}

func main() {
	configFile, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println("错误: 未找到 config.json。")
		os.Exit(1)
	}

	var config Config
	if err := json.Unmarshal(configFile, &config); err != nil {
		fmt.Println("错误: config.json 格式损坏:", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%d", config.IP, config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("启动服务失败: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("BetterLAN-server已启动，当前绑定 %s\n", addr)

	go func() {
		for {
			time.Sleep(3 * time.Second)
			globalMu.RLock()
			for _, grp := range groups {
				grp.mu.Lock()
				for name, p := range grp.presences {
					if time.Since(p.LastSeen) > 3*time.Second {
						delete(grp.presences, name)
					}
				}
				grp.mu.Unlock()
			}
			globalMu.RUnlock()
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil { continue }
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
	case "PING":
		conn.Write([]byte("PONG\n"))
		return

	case "PRESENCE":
		handlePresence := func(p []string) {
			if len(p) < 5 { return }
			grp, pwd, name, authStatus := p[1], p[2], p[3], p[4]
			clientLang := "en"
			if len(p) >= 6 { clientLang = p[5] }
			isZh := strings.Contains(strings.ToLower(clientLang), "zh")
			
			globalMu.Lock()
			group, exists := groups[grp]
			if !exists {
				group = &Group{
					password:        pwd,
					hosts:           make(map[string]net.Conn),
					pendingSessions: make(map[string]Session),
					presences:       make(map[string]*Presence),
				}
				groups[grp] = group
			}
			globalMu.Unlock()

			if group.password == pwd {
				ipStr, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
				getGeoAsync(ipStr)

				group.mu.Lock()
				group.presences[name] = &Presence{
					Name:       name,
					AuthStatus: authStatus,
					IP:         ipStr,
					LastSeen:   time.Now(),
				}

				var list []string
				for _, pres := range group.presences {
					geoStr := "Unknown"
					if val, ok := geoCache.Load(pres.IP); ok {
						info := val.(*GeoInfo)
						if isZh { geoStr = info.Zh } else { geoStr = info.En }
					}
					list = append(list, fmt.Sprintf("%s:%s:%s", pres.Name, pres.AuthStatus, geoStr))
				}
				group.mu.Unlock()

				response := "PRESENCE_RES|" + strings.Join(list, ",") + "\n"
				conn.Write([]byte(response))
			}
		}

		handlePresence(parts)
		for {
			nextLine, err := reader.ReadString('\n')
			if err != nil { break }
			nextParts := strings.Split(strings.TrimSpace(nextLine), "|")
			if len(nextParts) > 0 && nextParts[0] == "PRESENCE" {
				handlePresence(nextParts)
			} else {
				break
			}
		}
		conn.Close()

	case "LIST":
		if len(parts) >= 3 {
			grp, pwd := parts[1], parts[2]
			globalMu.RLock()
			group, exists := groups[grp]
			globalMu.RUnlock()
			if exists && group.password == pwd {
				group.mu.RLock()
				var hostNames []string
				for name := range group.hosts { hostNames = append(hostNames, name) }
				group.mu.RUnlock()
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
					presences:       make(map[string]*Presence),
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
			buf := make([]byte, 10)
			conn.Read(buf) 
			group.mu.Lock()
			delete(group.hosts, name)
			group.mu.Unlock()
		}
		conn.Close()

	case "GUEST_JOIN":
		if len(parts) >= 4 {
			grp, pwd, target := parts[1], parts[2], parts[3]
			globalMu.RLock()
			group, exists := groups[grp]
			globalMu.RUnlock()
			if exists && group.password == pwd {
				group.mu.Lock()
				hostConn, hostExists := group.hosts[target]
				if hostExists {
					sessionID := generateSessionID()
					group.pendingSessions[sessionID] = Session{guestConn: conn}
					hostConn.Write([]byte(fmt.Sprintf("INCOMING|%s\n", sessionID)))
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
			globalMu.RLock()
			group, exists := groups[grp]
			globalMu.RUnlock()
			if exists && group.password == pwd {
				group.mu.Lock()
				session, sessionExists := group.pendingSessions[sessionID]
				if sessionExists {
					delete(group.pendingSessions, sessionID)
					group.mu.Unlock()
					guestConn := session.guestConn
					guestConn.Write([]byte("OK\n"))
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
