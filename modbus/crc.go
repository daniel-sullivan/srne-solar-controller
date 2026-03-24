package modbus

// crcTable is the pre-computed CRC16 lookup table using polynomial 0xA001.
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

// CRC16 computes the MODBUS CRC16 checksum.
func CRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc = (crc >> 8) ^ crcTable[(crc^uint16(b))&0xFF]
	}
	return crc
}

// AppendCRC appends the CRC16 checksum to the data (low byte first).
func AppendCRC(data []byte) []byte {
	crc := CRC16(data)
	return append(data, byte(crc&0xFF), byte(crc>>8))
}

// ValidateCRC checks that the last two bytes of data are a valid CRC16.
func ValidateCRC(data []byte) bool {
	if len(data) < 3 {
		return false
	}
	return CRC16(data) == 0
}
