package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/mem"
)

var sizes = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

func FormatFileSize(s float64, base float64) string {
	unitsLimit := len(sizes)
	i := 0
	for s >= base && i < unitsLimit {
		s = s / base
		i++
	}

	f := "%.0f %s"
	if i > 1 {
		f = "%.2f %s"
	}

	return fmt.Sprintf(f, s, sizes[i])
}

func FormatFileSizeMb(s uint64) string {
	return FormatFileSize(float64(s), 1024)
}

func printMemStats(memResults *mem.VirtualMemoryStat) {
	fmt.Printf("Total: %v\nMemTotal - MemUsed: %v (%f%%)\nMemAvailable: %v (%f%%)\n", FormatFileSizeMb(memResults.Total), FormatFileSizeMb(memResults.Total-memResults.Used), 100-memResults.UsedPercent, FormatFileSizeMb(memResults.Available), float64(float64(memResults.Available)/float64(memResults.Total))*100)
}

func watchMemoryUsage(ctx context.Context, collectionInterval time.Duration) {
	ticker := time.NewTicker(collectionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _ = <-ticker.C:
			memResults, err := mem.VirtualMemory()
			if err != nil {
				log.Fatal(err)
			}
			printMemStats(memResults)
		}
	}
}

func allocateBytes(size int) []byte {
	ballast := make([]byte, size)
	for i := 0; i < len(ballast); i++ {
		ballast[i] = byte('A')
	}
	return ballast
}

func main() {
	useAvailableMem := flag.Bool("memAvailable", false, "Use <MemAvailable> instead of <Total - Free - Buffers - Cached>")
	mb := flag.Uint64("mb", 100, "Memory to allocate in MB (binary: 1MB = 1024*1024 bytes)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	go watchMemoryUsage(ctx, 20*time.Second)
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	memResults, err := mem.VirtualMemory()
	if err != nil {
		log.Fatal(err)
	}

	// Fill with "Total - Used" - 1MB mercy
	bytesToAllocate := memResults.Total - memResults.Used
	strategyFormula := "Total - Used"
	if *useAvailableMem {
		// Fill with "Available memory"
		bytesToAllocate = memResults.Available
		strategyFormula = "Available"
	}

	// remove some mercy MB
	bytesToAllocate -= *mb * 1024 * 1024

	printMemStats(memResults)
	fmt.Printf("PID: %d, ALLOCATING<%s - %d mercy MB>: %v\n", os.Getpid(), strategyFormula, *mb, FormatFileSizeMb(bytesToAllocate))

	var allocatedMemory []byte
	go func() {
		allocatedMemory = allocateBytes(int(bytesToAllocate))
	}()

	rcvSignal := <-done
	log.Printf("Exciting, received signal: %v\n", rcvSignal.String())
	debug.PrintStack()
	cancel()
	allocatedMemory[0] = 'B'
}
