// tiedot main entrance.
package main

import (
	"flag"
	"github.com/steveoc64/tiedot/db"
	"github.com/steveoc64/tiedot/srv/v3"
	"github.com/steveoc64/tiedot/tdlog"
	"log"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"
)

func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	var err error
	var defaultMaxprocs int
	if defaultMaxprocs, err = strconv.Atoi(os.Getenv("GOMAXPROCS")); err != nil {
		defaultMaxprocs = runtime.NumCPU() * 2
	}

	// Parse CLI parameters
	var mode, dir string
	var port, maxprocs, benchSize int
	var profile bool
	flag.StringVar(&mode, "mode", "", "[httpd|bench|bench2|example]")
	flag.StringVar(&dir, "dir", "", "database directory")
	flag.IntVar(&port, "port", 8080, "listening port number")
	flag.IntVar(&maxprocs, "gomaxprocs", defaultMaxprocs, "GOMAXPROCS")
	flag.IntVar(&benchSize, "benchsize", 400000, "Benchmark sample size")
	flag.BoolVar(&profile, "profile", false, "write profiler results to prof.out")
	flag.Parse()

	// User must specify a mode to run
	if mode == "" {
		flag.PrintDefaults()
		return
	}

	// Setup appropriate GOMAXPROCS parameter
	runtime.GOMAXPROCS(maxprocs)
	log.Printf("GOMAXPROCS is set to %d", maxprocs)
	if maxprocs < runtime.NumCPU() {
		tdlog.Printf("GOMAXPROCS (%d) is less than number of CPUs (%d), this may affect performance. You can change it via environment variable GOMAXPROCS or by passing CLI parameter -gomaxprocs", maxprocs, runtime.NumCPU())
	}

	// Start profiler if enabled
	if profile {
		resultFile, err := os.Create("perf.out")
		if err != nil {
			log.Panicf("Cannot create profiler result file %s", resultFile)
		}
		pprof.StartCPUProfile(resultFile)
		defer pprof.StopCPUProfile()
	}

	switch mode {
	case "httpd": // Run HTTP API server
		if dir == "" {
			tdlog.Fatal("Please specify database directory, for example -dir=/tmp/db")
		}
		if port == 0 {
			tdlog.Fatal("Please specify port number, for example -port=8080")
		}
		db, err := db.OpenDB(dir)
		if err != nil {
			tdlog.Fatal(err)
		}
		v3.Start(db, port)
	case "example": // Run embedded usage examples
		embeddedExample()
	case "bench": // Benchmark scenarios
		benchmark(benchSize)
	case "bench2":
		benchmark2(benchSize)
	default:
		flag.PrintDefaults()
		return
	}
}
