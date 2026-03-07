// recover searches every possible seed index to find which one produced a
// given nano_ address from a given master seed.
//
// Usage:
//
//	go run ./cmd/recover \
//	  -seed dfee6fad32b5e0599387a47acd7a27cb3bb4c9fc492ccd995b0f5c20b60bbc72 \
//	  -address nano_3iow9aycnccdd6zip6mzduq1qzc6t9xagictim8sgsog9p9md3s5y4yzu685
//
// When found it prints the matching index so you can import the seed into
// Nault (nault.cc) and navigate to that account number to access the funds.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"

	"github.com/allanabrahao/nanomultiplayer/internal/nano"
)

func main() {
	seedHex := flag.String("seed", "", "master seed as 64 hex characters")
	target := flag.String("address", "", "nano_ address to find")
	flag.Parse()

	if *seedHex == "" || *target == "" {
		flag.Usage()
		log.Fatal("both -seed and -address are required")
	}

	seed, err := hex.DecodeString(*seedHex)
	if err != nil || len(seed) != 32 {
		log.Fatalf("seed must be 64 hex characters (32 bytes), got %d bytes", len(seed))
	}

	fmt.Printf("Searching all %d possible indices…\n", math.MaxUint32)
	fmt.Println("This may take a few minutes. Press Ctrl-C to abort.")

	for i := uint64(0); i <= math.MaxUint32; i++ {
		addr, err := nano.DeriveAddress(seed, uint32(i))
		if err != nil {
			log.Fatalf("derive error at index %d: %v", i, err)
		}
		if addr == *target {
			fmt.Printf("\nFound!\n  seed index : %d\n  address    : %s\n\nTo access the funds:\n  1. Open https://nault.cc\n  2. Import wallet — paste the seed\n  3. Navigate to account #%d\n", i, addr, i)
			return
		}
		if i%1_000_000 == 0 && i > 0 {
			fmt.Printf("  checked %dm indices…\n", i/1_000_000)
		}
	}

	fmt.Println("\nNot found — the address was not derived from this seed.")
}
