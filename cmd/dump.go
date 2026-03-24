package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/daniel-sullivan/srne-solar-controller/modbus"
	"github.com/daniel-sullivan/srne-solar-controller/register"
	"github.com/spf13/cobra"
)

var dumpCmd = &cobra.Command{
	Use:   "dump <group>",
	Short: "Dump a register group with human-readable output",
	Long: func() string {
		groups := register.AllGroups()
		names := make([]string, 0, len(groups))
		for name := range groups {
			names = append(names, name)
		}
		sort.Strings(names)
		return fmt.Sprintf("Dump a named register group.\n\nAvailable groups: all, faults, %s", strings.Join(names, ", "))
	}(),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := newClient()
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()

		session := modbus.NewSession(client)
		groupName := args[0]

		if groupName == "faults" {
			return dumpFaultHistory(session)
		}

		if groupName == "all" {
			for _, entry := range register.OrderedGroups() {
				if err := dumpGroup(session, entry.Group); err != nil {
					fmt.Printf("  ERROR: %v\n", err)
				}
				fmt.Println()
			}
			if err := dumpFaultHistory(session); err != nil {
				fmt.Printf("  ERROR: %v\n", err)
			}
			return nil
		}

		groups := register.AllGroups()
		group, ok := groups[groupName]
		if !ok {
			names := make([]string, 0, len(groups))
			for name := range groups {
				names = append(names, name)
			}
			sort.Strings(names)
			return fmt.Errorf("unknown group %q, available: all, faults, %s", groupName, strings.Join(names, ", "))
		}

		return dumpGroup(session, group)
	},
}

func init() {
	rootCmd.AddCommand(dumpCmd)
}

func dumpGroup(session *modbus.Session, group register.Group) error {
	fmt.Printf("=== %s ===\n", group.Name)
	if len(group.Registers) == 0 {
		return nil
	}

	const maxRegsPerRead = 32
	if err := bulkRead(session, group, maxRegsPerRead); err != nil {
		return err
	}

	for _, reg := range group.Registers {
		regCount := int(reg.RegCount())
		regValues := make([]uint16, regCount)
		ok := true
		for i := 0; i < regCount; i++ {
			v, err := session.Lookup(reg.Address + uint16(i))
			if err != nil {
				ok = false
				break
			}
			regValues[i] = v
		}

		if !ok {
			if reg.Optional {
				continue // silently skip unavailable optional registers
			}
			fmt.Printf("  0x%04X %-35s <read error>\n", reg.Address, reg.Name)
			continue
		}

		formatted := register.FormatValue(reg, regValues, session.Lookup)

		extra := enumLabel(reg, regValues)
		if extra != "" {
			formatted = fmt.Sprintf("%s (%s)", formatted, extra)
		}

		fmt.Printf("  0x%04X %-35s %s\n", reg.Address, reg.Name, formatted)
	}
	return nil
}

// bulkRead reads all registers in a group in batched reads and stores them in the session cache.
// Optional registers are read in their own spans so failures don't affect required registers.
func bulkRead(session *modbus.Session, group register.Group, maxPerRead int) error {
	type span struct {
		start, end uint16
		optional   bool
	}
	var spans []span

	for _, reg := range group.Registers {
		end := reg.Address + reg.RegCount()
		// Optional registers always get their own span to isolate failures
		if reg.Optional {
			spans = append(spans, span{reg.Address, end, true})
			continue
		}
		if len(spans) > 0 {
			last := &spans[len(spans)-1]
			gap := int(reg.Address) - int(last.end)
			if !last.optional && gap >= 0 && gap <= 4 && int(end-last.start) <= maxPerRead {
				last.end = end
				continue
			}
		}
		spans = append(spans, span{reg.Address, end, false})
	}

	for _, s := range spans {
		count := s.end - s.start
		values, err := session.ReadRegisters(s.start, count)
		if err != nil {
			if !s.optional {
				return fmt.Errorf("read 0x%04X-0x%04X: %w", s.start, s.end-1, err)
			}
			// Optional register not available on this device — skip silently
			continue
		}
		session.Store(s.start, values)
	}
	return nil
}

func dumpFaultHistory(session *modbus.Session) error {
	fmt.Println("=== Fault History ===")

	count := 0
	for i := 0; i < register.FaultRecordCount; i++ {
		addr := uint16(register.FaultHistoryBase) + uint16(i*register.FaultRecordSize)
		values, err := session.ReadRegisters(addr, uint16(register.FaultRecordSize))
		if err != nil {
			return fmt.Errorf("read fault record %d at 0x%04X: %w", i, addr, err)
		}

		record := register.ParseFaultRecord(i, values)
		if record.IsEmpty() {
			continue
		}

		fmt.Println(register.FormatFaultRecord(record))
		count++
	}

	if count == 0 {
		fmt.Println("  (no faults recorded)")
	}
	return nil
}

func enumLabel(reg register.Register, values []uint16) string {
	if len(values) == 0 {
		return ""
	}
	switch reg.Address {
	case register.AddrMachineState:
		return register.MachineState(values[0])
	case register.AddrChargeStatus:
		return register.ChargeStatus(values[0])
	case register.AddrBatteryType:
		return register.BatteryType(values[0])
	case register.AddrOutputPriority:
		return register.OutputPriority(values[0])
	case register.AddrChargerPriority:
		return register.ChargerPriority(values[0])
	}
	return ""
}
