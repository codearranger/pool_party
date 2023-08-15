package main

import (
	"flag"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var connectionPool []*net.TCPConn
var poolLock sync.Mutex
var ips []net.IP

func createConnection(ip net.IP, port int) {
	addr := net.TCPAddr{IP: ip, Port: port}
	conn, err := net.DialTCP("tcp", nil, &addr)
        if err != nil {
                log.Printf("Failed to connect to %v: %v", addr, err)
                return
        }
        log.Printf("Connected to %v", addr)

        // Enable TCP keep-alive
        if err := conn.SetKeepAlive(true); err != nil {
                log.Printf("Failed to enable keep-alive for %v: %v", conn.RemoteAddr(), err)
                conn.Close()
                return
        }

        // Set the keep-alive period
        keepAlivePeriod := 3 * time.Second // You can adjust this value
        if err := conn.SetKeepAlivePeriod(keepAlivePeriod); err != nil {
                log.Printf("Failed to set keep-alive period for %v: %v", conn.RemoteAddr(), err)
                conn.Close()
                return
        }

/*
        // Disable Nagle's algorithm to reduce latency
        if err := conn.SetNoDelay(true); err != nil {
                log.Printf("Failed to disable Nagle's algorithm for %v: %v", conn.RemoteAddr(), err)
                conn.Close()
                return
        }
*/

        poolLock.Lock()
        connectionPool = append(connectionPool, conn)
        poolLock.Unlock()
}

func getConnectionFromPool() *net.TCPConn {
        poolLock.Lock()
        defer poolLock.Unlock()

        if len(connectionPool) == 0 {
                log.Println("No connections available in the pool")
                return nil
        }

        log.Println("Current pool members:")
        for i, conn := range connectionPool {
                log.Printf("  Member %d: %v", i, conn.RemoteAddr())
        }

        conn := connectionPool[rand.Intn(len(connectionPool))]
        log.Printf("Retrieved connection from pool: %v", conn.RemoteAddr())
        return conn
}

func removeFromPool(conn *net.TCPConn) {
	poolLock.Lock()
	defer poolLock.Unlock()

	var ip net.IP
	var port int
	for i, c := range connectionPool {
		if c == conn {
			ip = c.RemoteAddr().(*net.TCPAddr).IP
			port = c.RemoteAddr().(*net.TCPAddr).Port
			log.Printf("Removing connection from pool: %v", conn.RemoteAddr())
			connectionPool = append(connectionPool[:i], connectionPool[i+1:]...)
			break
		}
	}

	// Recreate the connection using the same IP and port
	if ip != nil {
		log.Printf("Recreating connection to: %v", ip)
		go createConnection(ip, port)
	}
}

func initializePool(target string) {
	hostPort := strings.Split(target, ":")
	host := hostPort[0]
	port, err := strconv.Atoi(hostPort[1])
	if err != nil {
		log.Fatalf("Invalid port in target %s: %v", target, err)
	}

	ips, err = net.LookupIP(host)
	if err != nil {
		panic(err)
	}

	log.Printf("Found %d IPs for host %s", len(ips), host)
	for _, ip := range ips {
		createConnection(ip, port)
	}
}


func forward(src, dst net.Conn) {
        defer src.Close()
        defer dst.Close()

        buf := make([]byte, 1024)
        for {   
                n, err := src.Read(buf)
                if err != nil {
                        if err != io.EOF {
                                log.Printf("Failed to read from %v: %v", src.RemoteAddr(), err)
                        }
                        if tcpConn, ok := dst.(*net.TCPConn); ok {
                                removeFromPool(tcpConn)
                        }
                        return
                }

                _, err = dst.Write(buf[:n])
                if err != nil {
                        log.Printf("Failed to write to %v: %v", dst.RemoteAddr(), err)
                        if tcpConn, ok := dst.(*net.TCPConn); ok {
                                removeFromPool(tcpConn)
                        }
                        return
                }
        }
}

func handleClient(conn net.Conn) {
        poolConn := getConnectionFromPool()
        if poolConn == nil {
                log.Println("Failed to get connection from pool")
                conn.Close()
                return
        }

        go forward(conn, poolConn)
        go forward(poolConn, conn)
}

func main() {
	target := flag.String("target", "mainnet-pociot.helium.io:9080", "The target host and port to connect to")
	listenAddr := flag.String("listen", "127.0.0.1:9080", "The IP and port to listen on")

	flag.Parse() // Parse the command-line arguments

	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)

	rand.Seed(time.Now().UnixNano()) // Initialize random seed
	initializePool(*target)

	tcpAddr, err := net.ResolveTCPAddr("tcp4", *listenAddr)
	if err != nil {
		panic(err)
	}

	listener, err := net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		panic(err)
	}
	log.Printf("Listening on %v", listener.Addr())
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Failed to accept connection:", err)
			time.Sleep(1 * time.Second)
			continue
		}
		log.Printf("Accepted client connection: %v", conn.RemoteAddr())
		go handleClient(conn)
	}
}
