package cmd

import (
	"fmt"

	"github.com/daniel-sullivan/srne-solar-controller/register"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Display inverter product information",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		defer client.Close()

		fmt.Println("=== Product Info ===")

		// Read product info: 0x000A through 0x001D (20 registers)
		vals, err := client.ReadRegisters(0x000A, 20)
		if err != nil {
			return fmt.Errorf("read product info: %w", err)
		}

		off := func(addr uint16) int { return int(addr - 0x000A) }

		productTypes := map[uint16]string{
			0: "Controller",
			3: "Inverter",
			4: "Integrated Inverter Controller",
			5: "Mains-Frequency Off-Grid",
		}
		pt := vals[off(0x000B)]
		if s, ok := productTypes[pt]; ok {
			fmt.Printf("  Product Type:      %s\n", s)
		} else {
			fmt.Printf("  Product Type:      %d\n", pt)
		}

		// Model string (0x000C-0x0013, 8 registers)
		model := register.FormatValue(register.Register{Type: register.ASCII, Count: 8}, vals[off(0x000C):off(0x000C)+8], nil)
		if model != "" {
			fmt.Printf("  Model:             %s\n", model)
		}

		sw1 := vals[off(0x0014)]
		sw2 := vals[off(0x0015)]
		fmt.Printf("  SW Version CPU1:   V%d.%02d\n", sw1/100, sw1%100)
		fmt.Printf("  SW Version CPU2:   V%d.%02d\n", sw2/100, sw2%100)

		hw1 := vals[off(0x0016)]
		hw2 := vals[off(0x0017)]
		fmt.Printf("  HW Version (Ctrl): V%d.%02d\n", hw1/100, hw1%100)
		fmt.Printf("  HW Version (Pwr):  V%d.%02d\n", hw2/100, hw2%100)

		fmt.Printf("  RS485 Address:     %d\n", vals[off(0x001A)])
		fmt.Printf("  Model Code:        %d\n", vals[off(0x001B)])

		pv := vals[off(0x001C)]
		fmt.Printf("  Protocol Version:  V%d.%02d\n", pv/100, pv%100)

		// Try serial number (0x0035-0x0048, 20 registers)
		if snVals, err := client.ReadRegisters(0x0035, 20); err == nil {
			sn := register.FormatValue(register.Register{Type: register.ASCII, Count: 20}, snVals, nil)
			if sn != "" {
				fmt.Printf("  Serial Number:     %s\n", sn)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
