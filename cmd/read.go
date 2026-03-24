package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var readCount uint16

var readCmd = &cobra.Command{
	Use:   "read <address> [address...]",
	Short: "Read registers by address",
	Long:  "Read one or more holding registers by hex address (e.g., 0x0100).",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		defer client.Close()

		for _, arg := range args {
			addr, err := parseAddr(arg)
			if err != nil {
				return fmt.Errorf("invalid address %q: %w", arg, err)
			}

			values, err := client.ReadRegisters(addr, readCount)
			if err != nil {
				return fmt.Errorf("read 0x%04X: %w", addr, err)
			}

			for i, v := range values {
				fmt.Printf("0x%04X: %d (0x%04X)\n", addr+uint16(i), v, v)
			}
		}
		return nil
	},
}

func init() {
	readCmd.Flags().Uint16VarP(&readCount, "count", "n", 1, "Number of registers to read per address")
	rootCmd.AddCommand(readCmd)
}

func parseAddr(s string) (uint16, error) {
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}
