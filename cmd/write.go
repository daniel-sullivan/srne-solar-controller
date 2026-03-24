package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/daniel-sullivan/srne-solar-controller/register"
	"github.com/spf13/cobra"
)

var writeForce bool

var writeCmd = &cobra.Command{
	Use:   "write <address> <value>",
	Short: "Write a value to a register",
	Long:  "Write a single register by hex address. Value can be decimal or hex (0x prefix).\nUse --force to skip confirmation for unknown or read-only registers.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, err := parseAddr(args[0])
		if err != nil {
			return fmt.Errorf("invalid address %q: %w", args[0], err)
		}

		value, err := parseValue(args[1])
		if err != nil {
			return fmt.Errorf("invalid value %q: %w", args[1], err)
		}

		reg, known := writeRegisterMap()[addr]
		if !writeForce {
			if !known {
				fmt.Printf("WARNING: 0x%04X is not in the register map.\n", addr)
				if !confirmPrompt("Write anyway?") {
					return fmt.Errorf("aborted")
				}
			} else if reg.Access == register.ReadOnly {
				fmt.Printf("WARNING: 0x%04X (%s) is read-only.\n", addr, reg.Name)
				if !confirmPrompt("Write anyway?") {
					return fmt.Errorf("aborted")
				}
			}
		}

		client, err := newClient()
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()

		if err := client.WriteSingleRegister(addr, value); err != nil {
			return fmt.Errorf("write 0x%04X: %w", addr, err)
		}

		name := fmt.Sprintf("0x%04X", addr)
		if known {
			name = reg.Name
		}
		fmt.Printf("wrote %s (0x%04X) = %d (0x%04X)\n", name, addr, value, value)
		return nil
	},
}

func init() {
	writeCmd.Flags().BoolVarP(&writeForce, "force", "f", false, "Skip confirmation for unknown or read-only registers")
	rootCmd.AddCommand(writeCmd)
}

func writeRegisterMap() map[uint16]register.Register {
	m := make(map[uint16]register.Register)
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
			m[r.Address] = r
		}
	}
	return m
}

func confirmPrompt(msg string) bool {
	fmt.Printf("%s [y/N] ", msg)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(strings.ToLower(scanner.Text())) == "y"
	}
	return false
}

func parseValue(s string) (uint16, error) {
	base := 10
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		s = s[2:]
		base = 16
	}
	v, err := strconv.ParseUint(s, base, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}
