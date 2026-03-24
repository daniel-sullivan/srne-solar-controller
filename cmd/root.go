package cmd

import (
	"fmt"
	"os"

	"github.com/daniel-sullivan/srne-solar-controller/interfaces/solarman"
	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/spf13/cobra"
)

var (
	host    string
	port    int
	slaveID uint8
	serial  uint32
	debug   bool
)

var rootCmd = &cobra.Command{
	Use:   "srne",
	Short: "SRNE solar inverter CLI",
	Long:  "CLI tool for communicating with SRNE ASF-series inverters via Solarman V5 wifi dongles.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&host, "host", "", "Solarman dongle IP address (required)")
	rootCmd.PersistentFlags().IntVar(&port, "port", 8899, "Solarman dongle port")
	rootCmd.PersistentFlags().Uint8Var(&slaveID, "slave", 1, "MODBUS slave ID")
	rootCmd.PersistentFlags().Uint32Var(&serial, "serial", 0, "Dongle serial number")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Print raw frame hex dumps")
	_ = rootCmd.MarkPersistentFlagRequired("host")
}

func newClient() (modbus.Client, error) {
	client := solarman.NewClient(host, port, serial, slaveID)
	client.Debug = debug
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return client, nil
}
