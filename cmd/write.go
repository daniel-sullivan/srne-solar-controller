package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var writeCmd = &cobra.Command{
	Use:   "write <address> <value>",
	Short: "Write a value to a register",
	Long:  "Write a single register by hex address. Value can be decimal or hex (0x prefix).",
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

		client, err := newClient()
		if err != nil {
			return err
		}
		defer client.Close()

		if err := client.WriteSingleRegister(addr, value); err != nil {
			return fmt.Errorf("write 0x%04X: %w", addr, err)
		}

		fmt.Printf("wrote 0x%04X = %d (0x%04X)\n", addr, value, value)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(writeCmd)
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
