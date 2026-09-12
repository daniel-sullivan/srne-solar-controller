// Package interpack implements decoding, settings queries, and stream framing
// for the JBD UP16S family battery management system (BMS) inter-pack bus.
//
// Earlier documentation and working notes referred to this protocol as PACE
// P16S100A; however, live query responses containing UP16S015 and JBD device
// identifiers, along with exact frame length and CRC-16/MODBUS verification,
// confirm that the binary bus protocol belongs to the JBD UP16S family.
//
// # Physical Bus and Topology
//
// The inter-pack bus is a daisy-chained RS485 communication link running at
// 19200 baud, 8N1. On this bus, the master pack or controller polls slave packs
// with 10-byte poll requests, and each addressed pack responds with a 126-byte
// telemetry data frame.
//
// # Telemetry Frame Layout (Function Code 0x45)
//
// The 10-byte telemetry poll request format is:
//   - Byte 0: Echoed target pack address (ADR: 0x01 master, 0x02-0x0F slaves, 0x00 broadcast)
//   - Byte 1: Command byte 0x45 ('E')
//   - Bytes 2-5: Constant header 0x00, 0x00, 0x00, 0x54
//   - Bytes 6-7: Reserved 0x00, 0x00
//   - Bytes 8-9: CRC-16/MODBUS checksum (little-endian)
//
// The 126-byte telemetry reply layout is:
//   - Byte 0: Echoed pack address
//   - Byte 1: Command byte 0x45
//   - Bytes 2-5: Constant header 0x00, 0x00, 0x00, 0x54
//   - Byte 6: Reserved byte 0x00
//   - Byte 7: Frame type byte 0x74 (telemetry response)
//   - Bytes 8-9: Cell count slot width (big-endian 0x0010 = 16 cells)
//   - Bytes 10-41: 16 individual cell voltages (big-endian uint16, millivolts)
//   - Bytes 42-43: Max cell voltage (big-endian uint16, mV)
//   - Bytes 44-45: Min cell voltage (big-endian uint16, mV)
//   - Bytes 46-47: Pack voltage (big-endian uint16, centivolts, e.g. 5353 = 53.53 V)
//   - Bytes 48-51: Pack current (4 bytes, signed big-endian)
//   - Bytes 52-55: 2 BMS temperature readings (big-endian uint16, 0.1 Kelvin units)
//   - Bytes 56-57: Full pack capacity (big-endian uint16, 10 mAh units, e.g. 10000 = 100.00 Ah)
//   - Bytes 58-59: Remaining capacity (big-endian uint16, 10 mAh units, e.g. 9641 = 96.41 Ah)
//   - Bytes 60-61: Cycle count (big-endian uint16)
//   - Bytes 62-63: State of Charge (SOC, big-endian uint16, percentage 0-100%)
//   - Bytes 64-65: State of Health (SOH, big-endian uint16, percentage 0-100%)
//   - Bytes 66-69: Level-2 protection fault bitmask (uint32 big-endian)
//   - Bytes 70-71: MOS state bitmask (uint16 big-endian)
//   - Bytes 72-73: Level-1 warning bitmask (uint16 big-endian)
//   - Bytes 74-85: Serial / batch identification lot code
//   - Bytes 86-103: Reserved block (18 bytes)
//   - Bytes 104-109: Limits / counter fields
//   - Bytes 110-113: 2 environment/MOSFET temperatures (big-endian uint16, 0.1 K units)
//   - Bytes 114-115: Count / type fields
//   - Bytes 116-123: 4 cell-strap temperatures (big-endian uint16, 0.1 K units)
//   - Bytes 124-125: CRC-16/MODBUS checksum (little-endian)
//
// Remaining unverified bitfields across firmware variants are preserved as raw
// data and explicitly flagged as unresolved.
//
// # Settings Frame Layout (Function Code 0x78)
//
// Read-only settings queries use function code 0x78.
//
// The 10-byte request format is:
//   - Byte 0: Target pack address
//   - Byte 1: Function code 0x78
//   - Bytes 2-3: Start register address (big-endian uint16)
//   - Bytes 4-5: End register address (big-endian uint16)
//   - Bytes 6-7: Reserved 0x00, 0x00
//   - Bytes 8-9: CRC-16/MODBUS checksum (little-endian)
//
// The reply structure contains an 8-byte header, variable payload, and 2-byte CRC:
//   - Byte 0: Echoed pack address
//   - Byte 1: Function code 0x78
//   - Bytes 2-3: Echoed start register (big-endian uint16)
//   - Bytes 4-5: Echoed end register (big-endian uint16)
//   - Bytes 6-7: Payload length (big-endian uint16, N bytes)
//   - Bytes 8 to (8+N-1): Payload data (N bytes)
//   - Bytes (8+N) to (8+N+1): CRC-16/MODBUS checksum (little-endian)
//
// Two settings register blocks are verified and decoded:
//
// 1. Balancing & Full Charge Block (start 0x1C00, end 0x1CA0):
//   - Reply payload: 134 bytes (total frame 144 bytes)
//   - Payload offsets 4:6: Balance start voltage (big-endian uint16, millivolts)
//   - Payload offsets 6:8: Balance delta voltage threshold (big-endian uint16, millivolts)
//   - Payload offsets 12:14: Full charge voltage (big-endian uint16, centivolts)
//
// 2. Over-Voltage Protection Block (start 0x1800, end 0x1900):
//   - Reply payload: 208 bytes (total frame 218 bytes)
//   - Payload offsets 0:2: Cell OVP alarm threshold (big-endian uint16, millivolts)
//   - Payload offsets 6:8: Cell OVP protection cutoff (big-endian uint16, millivolts)
//   - Payload offsets 24:26: Pack OVP alarm threshold (big-endian uint16, centivolts)
//   - Payload offsets 30:32: Pack OVP protection cutoff (big-endian uint16, centivolts)
//
// # Safety and Operational Boundaries
//
// This package is strictly read-only: it constructs no write commands, applies
// no charging control, and does not mutate BMS parameters.
//
// Safe or healthy battery operation is not asserted until all 10 unique physical
// pack identities and their alarm/protection statuses are verified and accounted for.
//
// Address 0 represents an unaddressed or broadcast responder whose physical
// identity is ambiguous; Store flags it with IdentityUncertain.
//
// All serial numbers and hardware identifiers are redacted from documentation
// and test vectors to protect deployment privacy.
package interpack
