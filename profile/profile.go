package profile

import (
	"encoding/json"
	"fmt"
	"runtime"
	"time"
)

type Profile struct {
	Alloc        uint64
	TotalAlloc   uint64
	Sys          uint64
	Mallocs      uint64
	Frees        uint64
	LiveObjects  uint64
	PauseTotalNs uint64

	NumGC        uint32
	NumGoroutine int
}

func NewProfile(duration time.Duration) {
	var (
		m   Profile
		rtm runtime.MemStats
	)

	go func() {
		for {
			<-time.After(duration)

			// Read full mem stats
			runtime.ReadMemStats(&rtm)

			// Number of goroutines
			m.NumGoroutine = runtime.NumGoroutine()

			// Misc memory stats
			m.Alloc = rtm.Alloc
			m.TotalAlloc = rtm.TotalAlloc
			m.Sys = rtm.Sys
			m.Mallocs = rtm.Mallocs
			m.Frees = rtm.Frees

			// Live objects = Mallocs - Frees
			m.LiveObjects = m.Mallocs - m.Frees

			// GC Stats
			m.PauseTotalNs = rtm.PauseTotalNs
			m.NumGC = rtm.NumGC

			// Just encode to json and print
			b, _ := json.Marshal(m)
			fmt.Println(string(b))
		}
	}()
}
