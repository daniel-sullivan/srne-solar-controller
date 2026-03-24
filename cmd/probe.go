package cmd

import (
	"fmt"
	"strings"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
	"github.com/spf13/cobra"
)

var (
	probeShowAll bool
)

// Known probe ranges with descriptive names.
var probeRanges = []struct {
	name       string
	start, end uint16
}{
	{"P00 Product Info", 0x000A, 0x0050},
	{"P01 Battery/PV", 0x0100, 0x0140},
	{"P02 Inverter", 0x0200, 0x0240},
	{"P05 Battery Settings", 0xE000, 0xE060},
	{"P07 Inverter Settings", 0xE200, 0xE230},
	{"P08 Statistics", 0xF000, 0xF060},
}

var probeCmd = &cobra.Command{
	Use:   "probe [start] [end]",
	Short: "Probe register ranges to discover available registers",
	Long: `Reads every register in a range and classifies them as active (non-zero),
mapped (returns 0), or unmapped (returns error).

Without arguments, probes all known register ranges.
With arguments, probes a custom hex range: probe 0xE000 0xE060`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()

		// Build known register lookup for annotation
		known := buildKnownMap()

		if len(args) == 2 {
			start, err := parseAddr(args[0])
			if err != nil {
				return fmt.Errorf("invalid start address: %w", err)
			}
			end, err := parseAddr(args[1])
			if err != nil {
				return fmt.Errorf("invalid end address: %w", err)
			}
			return probeRange(client, "Custom", start, end, known)
		}

		for _, r := range probeRanges {
			if err := probeRange(client, r.name, r.start, r.end, known); err != nil {
				fmt.Printf("  ERROR: %v\n", err)
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	probeCmd.Flags().BoolVarP(&probeShowAll, "all", "a", false, "Show all registers including mapped-zero (default: only active and unmapped boundaries)")
	rootCmd.AddCommand(probeCmd)
}

const probeBatchSize = 16

func probeRange(client modbus.Client, name string, start, end uint16, known map[uint16]string) error {
	fmt.Printf("=== %s (0x%04X-0x%04X) ===\n", name, start, end-1)

	// Read all registers into a result map using batched reads with single-register fallback
	type probeResult struct {
		value uint16
		err   error
	}
	results := make(map[uint16]probeResult)

	for batchStart := start; batchStart < end; batchStart += probeBatchSize {
		batchEnd := batchStart + probeBatchSize
		if batchEnd > end {
			batchEnd = end
		}
		count := batchEnd - batchStart

		values, err := client.ReadRegisters(batchStart, count)
		if err == nil {
			for i, v := range values {
				results[batchStart+uint16(i)] = probeResult{value: v}
			}
			continue
		}

		// Batch failed — fall back to individual reads
		for addr := batchStart; addr < batchEnd; addr++ {
			vals, err := client.ReadRegisters(addr, 1)
			if err != nil {
				results[addr] = probeResult{err: err}
			} else {
				results[addr] = probeResult{value: vals[0]}
			}
		}
	}

	// Display results in address order
	var active, mapped, unmapped int
	prevState := ""

	for addr := start; addr < end; addr++ {
		r := results[addr]

		regName := known[addr]
		annotation := ""
		if regName != "" {
			annotation = fmt.Sprintf("  [%s]", regName)
		}

		if r.err != nil {
			unmapped++
			state := "unmapped"
			if prevState != state {
				fmt.Printf("  0x%04X: UNMAPPED (%v)%s\n", addr, shortErr(r.err), annotation)
			}
			prevState = state
			continue
		}

		raw := r.value
		if raw != 0 {
			active++
			fmt.Printf("  0x%04X: %6d (0x%04X)  ACTIVE%s\n", addr, raw, raw, annotation)
			prevState = "active"
		} else {
			mapped++
			if probeShowAll {
				fmt.Printf("  0x%04X: %6d (0x%04X)  mapped%s\n", addr, raw, raw, annotation)
			} else if prevState == "unmapped" || prevState == "" {
				fmt.Printf("  0x%04X: %6d (0x%04X)  mapped%s\n", addr, raw, raw, annotation)
			}
			prevState = "mapped"
		}
	}

	fmt.Printf("  --- %d active, %d mapped (zero), %d unmapped ---\n", active, mapped, unmapped)
	return nil
}

func shortErr(err error) string {
	s := err.Error()
	// Trim to just the modbus error if present
	if idx := strings.Index(s, "modbus error"); idx >= 0 {
		return s[idx:]
	}
	if strings.Contains(s, "timeout") {
		return "timeout"
	}
	return s
}

func buildKnownMap() map[uint16]string {
	known := make(map[uint16]string)
	// Add all register groups including product info
	allGroups := []register.Group{
		register.ProductInfo,
		register.BatteryData,
		register.InverterData,
		register.BatterySettings,
		register.TimedChargeDischarge,
		register.InverterSettings,
		register.Statistics,
	}
	for _, g := range allGroups {
		for _, r := range g.Registers {
			known[r.Address] = r.Name
		}
	}
	return known
}
