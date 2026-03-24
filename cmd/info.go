package cmd

import (
	"fmt"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
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
		defer func() { _ = client.Close() }()

		session := modbus.NewSession(client)

		fmt.Println("=== Product Info ===")

		// Read product info: 0x000A through 0x001D (20 registers)
		const base = register.AddrMaxVoltageRatedCurrent
		vals, err := session.ReadRegisters(base, 20)
		if err != nil {
			return fmt.Errorf("read product info: %w", err)
		}
		session.Store(base, vals)

		off := func(addr uint16) int { return int(addr - base) }

		productTypes := map[uint16]string{
			0: "Controller",
			3: "Inverter",
			4: "Integrated Inverter Controller",
			5: "Mains-Frequency Off-Grid",
		}
		pt := vals[off(register.AddrProductType)]
		if s, ok := productTypes[pt]; ok {
			fmt.Printf("  Product Type:      %s\n", s)
		} else {
			fmt.Printf("  Product Type:      %d\n", pt)
		}

		// Model string (8 registers) — some inverters leave this empty
		model := register.FormatValue(register.Register{Type: register.ASCII, Count: 8}, vals[off(register.AddrProductModel):off(register.AddrProductModel)+8], nil)
		if model != "" {
			fmt.Printf("  Model:             %s\n", model)
		} else {
			fmt.Printf("  Model Code:        %d\n", vals[off(register.AddrModelCode)])
		}

		sw1 := vals[off(register.AddrSoftwareVersionCPU1)]
		sw2 := vals[off(register.AddrSoftwareVersionCPU2)]
		fmt.Printf("  SW Version CPU1:   V%d.%02d\n", sw1/100, sw1%100)
		fmt.Printf("  SW Version CPU2:   V%d.%02d\n", sw2/100, sw2%100)

		hw1 := vals[off(register.AddrHardwareVersionControl)]
		hw2 := vals[off(register.AddrHardwareVersionPower)]
		fmt.Printf("  HW Version (Ctrl): V%d.%02d\n", hw1/100, hw1%100)
		fmt.Printf("  HW Version (Pwr):  V%d.%02d\n", hw2/100, hw2%100)

		fmt.Printf("  RS485 Address:     %d\n", vals[off(register.AddrRS485Address)])

		pv := vals[off(register.AddrProtocolVersion)]
		fmt.Printf("  Protocol Version:  V%d.%02d\n", pv/100, pv%100)

		// Try serial number (20 registers)
		if snVals, err := session.ReadRegisters(register.AddrSerialNumber, 20); err == nil {
			sn := register.FormatValue(register.Register{Type: register.ASCIILoByte, Count: 20}, snVals, nil)
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
