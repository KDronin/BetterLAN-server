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
	"time"
)

type Config struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

type Session struct {
	guestConn net.Conn
	createdAt time.Time
}

type Presence struct {
	Name       string
	AuthStatus string
	Ping       string
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
)

func generateSessionID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func main() {
	configFile, err := os.ReadFile("config.json")
	if err != nil {
		fmt.Println("ERROR: config.json not found.")
		os.Exit(1)
	}

	var config Config
	if err := json.Unmarshal(configFile, &config); err != nil {
		fmt.Println("ERROR: Failed to parse config.json:", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%d", config.IP, config.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("ERROR: Failed to start server: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("BetterLAN server started on %s\n", addr)

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
				for sessionID, session := range grp.pendingSessions {
					if time.Since(session.createdAt) > 60*time.Second {
						session.guestConn.Close()
						delete(grp.pendingSessions, sessionID)
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
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(30 * time.Second)
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(io.LimitReader(conn, 2048)) 
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
	conn.SetReadDeadline(time.Time{})

	switch action {
	case "PING":
		conn.Write([]byte("PONG\n"))
		return

	case "PRESENCE":
		handlePresence := func(p []string) {
			if len(p) < 5 { return }
			grp, pwd, name, authStatus := p[1], p[2], p[3], p[4]
			clientPing := "0"
			if len(p) >= 7 { clientPing = p[6] }

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
				group.mu.Lock()
				group.presences[name] = &Presence{
					Name:       name,
					AuthStatus: authStatus,
					Ping:       clientPing,
					LastSeen:   time.Now(),
				}

				var list []string
				for _, pres := range group.presences {
					list = append(list, fmt.Sprintf("%s:%s:%s", pres.Name, pres.AuthStatus, pres.Ping))
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
			} else { break }
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
			} else { conn.Write([]byte("ERROR|Auth Failed\n")) }
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
			
			buf := make([]byte, 1024)
			for {
				_, err := conn.Read(buf)
				if err != nil {
					break
				}
			}
			
			group.mu.Lock()
			if group.hosts[name] == conn {
				delete(group.hosts, name)
			}
			group.mu.Unlock()
		}
		conn.Close()

	case "GUEST_JOIN":
		if len(parts) >= 4 {
			grp, pwd, target := parts[1], parts[2], parts[3]
			guestName, authStatus, guestPubKey := "Unknown", "Offline", ""
			if len(parts) >= 7 {
				guestName, authStatus, guestPubKey = parts[4], parts[5], parts[6]
			}

			globalMu.RLock()
			group, exists := groups[grp]
			globalMu.RUnlock()
			if exists && group.password == pwd {
				group.mu.Lock()
				hostConn, hostExists := group.hosts[target]
				if hostExists {
					sessionID := generateSessionID()
					group.pendingSessions[sessionID] = Session{guestConn: conn, createdAt: time.Now()}
					hostConn.Write([]byte(fmt.Sprintf("INCOMING|%s|%s|%s|%s\n", sessionID, guestName, authStatus, guestPubKey)))
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
			hostPubKey := ""
			if len(parts) >= 5 {
				hostPubKey = parts[4]
			}

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
					guestConn.Write([]byte(fmt.Sprintf("OK|%s\n", hostPubKey)))
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