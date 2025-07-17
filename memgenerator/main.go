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
	const oneGB = 1 << 30 // 1GB in bytes = 1073741824

	for i := 0; i < len(ballast); i++ {
		ballast[i] = byte('A')
		// Sleep after each GB written
		if (i+1)%oneGB == 0 {
			time.Sleep(100 * time.Millisecond) // or any delay you want
		}
	}
	return ballast
}

// /proc/[pid]/oom_score_adj (since Linux 2.6.36)
// This file can be used to adjust the badness heuristic used to select which process gets killed in out-of-memory conditions.
// The badness heuristic assigns a value to each candidate task ranging from 0 (never kill) to 1000 (always kill) to determine which process is targeted. The units are roughly a proportion along that range of allowed memory the process may allocate from, based on an estimation of its current memory and swap use. For example, if a task is using all allowed memory, its badness score will be 1000. If it is using half of its allowed memory, its score will be 500.
func setBadnessEuristics(score int) error {
	fmt.Printf("Setting oom_score_adj to 1000 for process %d\n", os.Getpid())
	return os.WriteFile(fmt.Sprintf("/proc/%d/oom_score_adj", os.Getpid()), []byte(fmt.Sprintf("%d", score)), 0o644)
}

func main() {
	useAvailableMem := flag.Bool("memAvailable", false, "Use <MemAvailable> instead of <Total - Free - Buffers - Cached>")
	flag.Parse()

	// prioritize killing this process
	err := setBadnessEuristics(1000)
	if err != nil {
		log.Fatal(err)
	}

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

	// calculate default mercy bytes (3% of the bytes to allocate)
	mb := flag.Uint64("mercybytes", uint64(float64(bytesToAllocate)*0.03), "Mercy memory bytes to not allocate, defaults to 3% of the total to allocate")
	flag.Parse()

	// remove some mercy MB
	bytesToAllocate -= *mb

	printMemStats(memResults)
	fmt.Printf("PID: %d, ALLOCATING<%s - %s mercy>: %v\n", os.Getpid(), strategyFormula, FormatFileSizeMb(*mb), FormatFileSizeMb(bytesToAllocate))

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
