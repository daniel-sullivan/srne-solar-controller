package cmd

import (
	"fmt"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/solarman"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Passively scan dongle traffic and display all MODBUS responses",
	Long:  "Listens to the dongle's data stream and displays all MODBUS responses, useful for identifying register blocks from other services.",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := solarman.NewClient(host, port, serial, slaveID)
		client.Debug = debug
		if err := client.Connect(); err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		defer client.Close()

		fmt.Println("Scanning dongle traffic... (Ctrl+C to stop)")

		seen := make(map[int]int) // byte_count -> times_seen

		err := client.ScanFrames(func(frame []byte) bool {
			if len(frame) < 5 {
				return true
			}
			if !modbus.ValidateCRC(frame) {
				return true
			}
			if frame[1] != modbus.FuncReadHoldingRegisters {
				return true
			}

			byteCount := int(frame[2])
			regCount := byteCount / 2
			seen[byteCount]++

			fmt.Printf("\n--- Response #%d: %d registers (%d bytes) ---\n", seen[byteCount], regCount, byteCount)
			for i := 0; i < regCount; i++ {
				raw := uint16(frame[3+i*2])<<8 | uint16(frame[4+i*2])
				fmt.Printf("  [%2d] 0x%04X = %6d", i, raw, raw)
				if raw != 0 {
					fmt.Printf("  (×0.1=%.1f  ×0.01=%.2f)", float64(raw)*0.1, float64(raw)*0.01)
				}
				fmt.Println()
			}

			// Stop after seeing all 3 block types at least once
			if len(seen) >= 3 {
				allSeen := true
				for _, count := range seen {
					if count < 1 {
						allSeen = false
					}
				}
				if allSeen {
					return false
				}
			}
			return true
		})

		if err != nil {
			return err
		}

		fmt.Printf("\nBlock summary: ")
		for bc, count := range seen {
			fmt.Printf("%d bytes (%d regs) seen %dx  ", bc, bc/2, count)
		}
		fmt.Println()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
