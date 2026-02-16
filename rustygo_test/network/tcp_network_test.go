package rustygo_test

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
	"unsafe"

	rg "rustygo"
)

// -----------------------------
// Large packet struct (~10KB)
// -----------------------------
type LargePacket struct {
	Data [10 * 1024]byte
}

// -----------------------------
// Global sync.Pool for large packets (>10KB)
// -----------------------------
var largePacketPool = &sync.Pool{
	New: func() any {
		return &LargePacket{}
	},
}

// -----------------------------
// Hybrid allocator
// -----------------------------
func AllocPacket(size int, arena *rg.Arena) *LargePacket {
	if size <= 10*1024 { // small packet → arena
		mem := arena.Alloc(size)
		return (*LargePacket)(unsafe.Pointer(&mem[0]))
	}
	return largePacketPool.Get().(*LargePacket) // large packet → pool
}

func FreePacket(pkt *LargePacket) {
	if unsafe.Sizeof(*pkt) > 10*1024 {
		largePacketPool.Put(pkt)
	}
	// small packets are freed when Arena.Reset() is called
}

// -----------------------------
// TCP server that just echoes
// -----------------------------
func startTCPServer(addr string, wg *sync.WaitGroup) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer ln.Close()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 10*1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					_, _ = c.Write(buf[:n])
				}
			}(conn)
		}
	}()
	return ln, nil
}

// -----------------------------
// Find a free TCP port
// -----------------------------
func getFreePort() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	return ln.Addr().String(), nil
}

// -----------------------------
// Benchmark TCP traffic using hybrid allocator
// -----------------------------
func BenchmarkTCPNetworkHybrid(b *testing.B) {
	const (
		numClient  = 8
		numPackets = 5000
	)

	// pick a free port automatically
	addr, err := getFreePort()
	if err != nil {
		b.Fatal(err)
	}

	var wg sync.WaitGroup
	_, err = startTCPServer(addr, &wg)
	if err != nil {
		b.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // give server time to start

	fmt.Println("=== Benchmark TCP traffic with Hybrid Allocator ===")

	start := time.Now()
	var clientWG sync.WaitGroup
	clientWG.Add(numClient)

	for c := 0; c < numClient; c++ {
		go func() {
			defer clientWG.Done()
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				panic(err)
			}
			defer conn.Close()

			arena := rg.NewArena(numPackets * int(unsafe.Sizeof(LargePacket{})))

			for i := 0; i < numPackets; i++ {
				pkt := AllocPacket(int(unsafe.Sizeof(LargePacket{})), arena)

				// fill packet data without zeroing entire struct
				for j := range pkt.Data {
					pkt.Data[j] = byte(i % 256)
				}

				// send & receive
				_, _ = conn.Write(pkt.Data[:])
				resp := make([]byte, len(pkt.Data))
				_, _ = conn.Read(resp)

				// free if Pool
				FreePacket(pkt)
			}

			arena.Reset() // free all small packets at once
		}()
	}

	clientWG.Wait()
	elapsed := time.Since(start)
	fmt.Printf("[Hybrid] Processed %d packets across %d clients in %s\n",
		numPackets*numClient, numClient, elapsed)
	fmt.Println("=== Done ===")
}
