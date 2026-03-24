# Solarman V5 Protocol

Wraps MODBUS RTU frames inside a proprietary TCP envelope. Default port: **8899**.

Based on analysis of [davidrapan/ha-solarman](https://github.com/davidrapan/ha-solarman) `pysolarman/__init__.py`.

## Frame Structure

### Request Frame

| Offset | Size | Value | Description |
|--------|------|-------|-------------|
| 0 | 1 | `0xA5` | Start marker |
| 1-2 | 2 | LE u16 | Payload length (= 15 + modbus_frame_len) |
| 3 | 1 | `0x10` | Control code suffix |
| 4 | 1 | Control code | `0x45` = REQUEST |
| 5-6 | 2 | LE u16 | Sequence number (only low byte significant) |
| 7-10 | 4 | LE u32 | Dongle serial number |
| 11 | 1 | `0x02` | Frame type |
| 12-13 | 2 | `0x0000` | Sensor type placeholder |
| 14-25 | 12 | `0x00...` | Timestamp placeholders |
| 26..N | var | bytes | Inner MODBUS RTU frame (slave_id + PDU + CRC16) |
| N+1 | 1 | u8 | Checksum: sum(bytes[1..N]) & 0xFF |
| N+2 | 1 | `0x15` | End marker |

Total frame size = payload_length + 13 (11 header + 1 checksum + 1 end marker).

### Response Frame

Same header structure. Inner MODBUS RTU frame is at **bytes[25:-2]** (offset 25 to 2 before end).

Response control code = request code - 0x30. REQUEST 0x45 → response 0x15.

## Dongle Serial Number

**The serial number is required for the dongle to forward requests to the inverter.**

- Serial is a 4-byte LE uint32 at bytes 7-10
- Valid range: 2,147,483,648 to 4,294,967,295 (0x80000000-0xFFFFFFFF)
- If serial is 0, the dongle sends `0x00000000` — it will ACK the frame but NOT forward the MODBUS request to the inverter
- **Auto-detection**: Send a request with serial=0. The dongle responds with a short ACK frame containing the real serial at response bytes 7-10. Extract it and resend with the correct serial.
- The ha-solarman integration auto-detects serial from `response[7:11]` on first connection

## Sequence Numbers

- Initialized to a random value 0x01-0xFF
- Incremented by 1, wraps at 0xFF to 0x00
- Sent as LE u16 (high byte = 0x00)
- ha-solarman validates only the low byte of the response sequence

## Checksum

Sum of all bytes from offset 1 to second-to-last byte, `& 0xFF`.
This is NOT the MODBUS CRC16 — the inner frame has its own CRC.

## Connection Behavior

- **No handshake** — plain TCP connect, then send requests immediately
- **Unsolicited frames** — the dongle may send HEARTBEAT (0x47), HANDSHAKE (0x41), etc. at any time. ha-solarman acknowledges these with response_code = control_code - 0x30
- **Short ACK frames** — 29-byte responses with only 2 bytes at the inner frame position are dongle acknowledgements, not data. Wait for a longer frame with actual MODBUS data
- **Shared dongle** — other services (ha-solarman, cloud) may be polling the same dongle. Their responses will interleave with yours. Filter by MODBUS slave ID and CRC validation
- **Throttling** — ha-solarman uses 200ms minimum between frames

## Quirks

- **Double CRC**: Some responses have trailing `0x0000` after the real CRC. Detect by checking if inner CRC validates and response ends with `0x0000`, then strip 2 bytes.
- **Dongle discovery**: UDP broadcast to port 48899 with `WIFIKIT-214028-READ` or `HF-A11ASSISTHREAD`.
