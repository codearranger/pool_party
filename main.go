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
var targetHost string
var targetPort int
var initialPoolSize int

func emptyConnectionPool() {
	poolLock.Lock()
	defer poolLock.Unlock()

	for _, conn := range connectionPool {
		if conn != nil {
			conn.Close()
			log.Printf("Closed connection to %v", conn.RemoteAddr())
		}
	}

	connectionPool = []*net.TCPConn{}
	log.Println("Connection pool emptied.")
}

func createConnection(ip net.IP, port int) *net.TCPConn {
	addr := net.TCPAddr{IP: ip, Port: port}
	conn, err := net.DialTCP("tcp", nil, &addr)
	if err != nil {
		log.Printf("Failed to connect to %v: %v", addr, err)
		return nil
	}
	log.Printf("Connected to %v", addr)

	// Enable TCP keep-alive
	if err := conn.SetKeepAlive(true); err != nil {
		log.Printf("Failed to enable keep-alive for %v: %v", conn.RemoteAddr(), err)
		return nil
	}

	// Set the keep-alive period
	keepAlivePeriod := 30 * time.Second // Adjust as needed
	if err := conn.SetKeepAlivePeriod(keepAlivePeriod); err != nil {
		log.Printf("Failed to set keep-alive period for %v: %v", conn.RemoteAddr(), err)
		return nil
	}

	/*
        // Disable Nagle's algorithm to reduce latency
        if err := conn.SetNoDelay(true); err != nil {
                log.Printf("Failed to disable Nagle's algorithm for %v: %v", conn.RemoteAddr(), err)
                return nil
        }
	*/
	
	return conn
}

func initializePool(target string) {
	emptyConnectionPool()

	hostPort := strings.Split(target, ":")
	host := hostPort[0]
	port, err := strconv.Atoi(hostPort[1])
	if err != nil {
		log.Fatalf("Invalid port in target %s: %v", target, err)
	}

	// Update globals
	targetHost = host 
	targetPort = port

	ips, err = net.LookupIP(host)
	if err != nil {
		panic(err)
	}

	log.Printf("Found %d IPs for host %s", len(ips), host)
	for _, ip := range ips {
		if conn := createConnection(ip, port); conn != nil {
			poolLock.Lock()
			connectionPool = append(connectionPool, conn)
			poolLock.Unlock()
		}
	}

	initialPoolSize = len(connectionPool)
	log.Printf("Initialized pool with %d members", initialPoolSize)

}

func getConnectionFromPool() *net.TCPConn {
	poolLock.Lock()
	defer poolLock.Unlock()

	if len(connectionPool) < initialPoolSize {
		log.Printf("Pool size %d < initial %d, reinitializing", len(connectionPool), initialPoolSize) 
		initializePool(targetHost + ":" + strconv.Itoa(targetPort))
	}
	
	if len(connectionPool) == 0 {
		log.Println("No connections available in pool")
		initializePool(targetHost + ":" + strconv.Itoa(targetPort))
	}

	// Pick a random connection
	idx := rand.Intn(len(connectionPool))
	conn := connectionPool[idx]

	log.Printf("Retrieved connection from pool: %v", conn.RemoteAddr())
	return conn
}

func replaceConnectionInPool(badConn *net.TCPConn) {
	poolLock.Lock()
	defer poolLock.Unlock()

	if len(connectionPool) < initialPoolSize {
		initializePool(targetHost + ":" + strconv.Itoa(targetPort)) 
	}

	for i, conn := range connectionPool {
		if conn == badConn {
			// Only replace if it's a pool member
			newConn := createConnection(badConn.RemoteAddr().(*net.TCPAddr).IP, targetPort)
			if newConn != nil {
				connectionPool[i] = newConn
				log.Printf("Replaced bad connection to %v with %v",
					badConn.RemoteAddr(), newConn.RemoteAddr())
			} else {
				// Remove bad conn if replacement fails
				connectionPool = append(connectionPool[:i], connectionPool[i+1:]...)
				log.Printf("Removed bad connection %v", badConn.RemoteAddr())
			}
			return
		}
	}

	log.Printf("Connection %v not found in pool, not replacing", badConn.RemoteAddr())
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
				replaceConnectionInPool(src.(*net.TCPConn))
			}
			return
		}

		_, err = dst.Write(buf[:n])
		if err != nil {
			log.Printf("Failed to write to %v: %v", dst.RemoteAddr(), err)
			replaceConnectionInPool(dst.(*net.TCPConn))
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
