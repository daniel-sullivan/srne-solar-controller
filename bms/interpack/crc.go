package interpack

// crcTable is the pre-computed CRC-16 lookup table using polynomial 0xA001 (reflected 0x8005).
var crcTable [256]uint16

func init() {
	for i := 0; i < 256; i++ {
		crc := uint16(i)
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
		crcTable[i] = crc
	}
}

// CRC16 computes the CRC-16/MODBUS checksum over the provided data.
// Poly: 0xA001, Initial: 0xFFFF.
func CRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc = (crc >> 8) ^ crcTable[(crc^uint16(b))&0xFF]
	}
	return crc
}
