# SRNE MODBUS Register Map

Based on protocol V1.7 / V1.96 / V2.08 for ASF-series hybrid inverters.

## Sources

- **SRNE hybrid solar inverter MODBUS protocol V1.7 PDF** — from [shakthisachintha/SRNE-Hybrid-Inverter-Monitor](https://github.com/shakthisachintha/SRNE-Hybrid-Inverter-Monitor/tree/master/Resources)
- **SRNE protocol V1.96 with changelog** — from [krimsonkla/srne_ble_modbus](https://github.com/krimsonkla/srne_ble_modbus) `/resources/SRNE_Energy_Storage_Inverter_Protocol_v1.96.md`
- **SRNE protocol V2.08 PDF (latest known)** — from [phinix-org/SRNE-inverters-by-modbus-rs485](https://github.com/phinix-org/SRNE-inverters-by-modbus-rs485) `docs/EN-SRNE inverter MODBUS register address-V2.08.pdf`
- **HotNoob/PythonProtocolGateway** — V1.7 holding register CSV
- **phinix-org/SRNE-inverters-by-modbus-rs485** — ESPHome YAML (V2.08, most complete register definitions)
- **davidrapan/ha-solarman** — `inverter_definitions/srne_asf.yaml` (Home Assistant integration)
- **SRNE ASP 8-10kW User Manual V1.3** — [official PDF](https://www.srnesolar.com/userfiles/files/2025/11/28/ASP%20_8-10kW_U_All-in-one%20solar%20charge%20inverter_V1.3[20250514].pdf) — fault code table, wiring diagrams, specs

## Protocol Version History

The protocol version is reported at register 0x001C. Registers were added incrementally:

| Version | Date | Key Additions |
|---------|------|---------------|
| V1.00 | - | Base protocol, max 20 registers per read |
| V1.7 | Jun 2022 | Max 32 regs/read, dual PV input (0x010F-0x0111), 3-phase (0x022A-0x0237), timed charge/discharge (0xE026-0xE033), BMS settings |
| V1.94 | Oct 2023 | Per-section SOC cutoffs (0xE03B-0xE040) |
| V1.95 | Jan 2024 | Per-section voltage cutoffs (0xE041-0xE046), discharge power limits (0xE047-0xE049) |
| V1.96 | Jan 2024 | Charge power limits (0xE04A-0xE04C), charge source selection (0xE04D) |
| V2.08 | May 2025 | Latest known version, all registers present |

**Important**: The version history above applies to the **ASF series** protocol lineage. The **ASP series** (and possibly other newer model lines) uses an independent protocol version counter that restarted at V1.00. An ASP48100U200-H with SW V8.18 reports protocol V1.07, yet supports registers from ASF V1.7 (e.g., timed charge/discharge 0xE026-0xE033). The protocol version number cannot be used to reliably gate register availability across model lines.

**Approach**: Registers that may not exist on all models/firmware are probed at runtime. Reads returning "illegal address" (0x02) indicate the register is unsupported. Reads returning 0 in reserved address space (e.g., 0xE034-0xE03B on ASP V1.07) are addressable but non-functional.

## Protocol Basics

- Baud: 9600, 8N1 (RS485)
- Slave address: 1-254 (default 1), broadcast 0x00, universal 0xFF
- Max registers per request: 32 (V1.7), 20 (V1.00)
- CRC16 MODBUS polynomial 0xA001, low byte first
- 16-bit registers; 32-bit values = two registers, low word at lower address

## Function Codes

| Code | Purpose |
|------|---------|
| 0x03 | Read holding registers |
| 0x06 | Write single register |
| 0x10 | Write multiple registers |
| 0x78 | Reset to factory defaults (SRNE-specific) |
| 0x79 | Clear history (SRNE-specific) |

## Error Codes

| Code | Meaning |
|------|---------|
| 0x01 | Illegal function |
| 0x02 | Illegal address |
| 0x03 | Illegal data value |
| 0x04 | Operation failed |
| 0x05 | Password error |
| 0x06 | Frame error |
| 0x07 | Parameter read-only |
| 0x08 | Cannot change while running |
| 0x09 | Password protection locked |
| 0x0A | Length error |
| 0x0B | Permission denied |

## P00: Product Information (0x000A-0x0048) — Read-only

| Address | Name | Notes |
|---------|------|-------|
| 0x000A | Max voltage / rated charge current | High byte=voltage code, low byte=current code |
| 0x000B | Product type | 0=Controller, 3=Inverter, 4=Integrated, 5=Mains-freq |
| 0x000C-0x0013 | Product model string | 8 regs, ASCII |
| 0x0014-0x0015 | Software version (CPU1/CPU2) | e.g. 818 = V8.18 |
| 0x0016-0x0017 | Hardware version | Control board, power board |
| 0x0018-0x0019 | Product SN | |
| 0x001A | RS485 address | |
| 0x001B | Model code | |
| 0x001C-0x001D | Protocol version | |
| 0x001E-0x001F | Date of manufacture | Packed year/month, day/hour |
| 0x0035-0x0048 | Product SN string | 20 regs ASCII |

## P01: Controller/Battery Data (0x0100-0x0111) — Read-only

| Address | Name | Scale | Unit | Signed |
|---------|------|-------|------|--------|
| 0x0100 | Battery SOC | 1 | % | No |
| 0x0101 | Battery voltage | 0.1 | V | No |
| 0x0102 | Battery current | 0.1 | A | Yes (+charge/-discharge) |
| 0x0103 | Temps (hi=controller, lo=battery) | 1 | C | Yes (packed bytes) |
| 0x0104 | DC load voltage | 0.1 | V | No |
| 0x0105 | DC load current | 0.01 | A | No |
| 0x0106 | DC load power | 1 | W | No |
| 0x0107 | PV1 voltage | 0.1 | V | No |
| 0x0108 | PV1 current | 0.1 | A | No |
| 0x0109 | PV1 charge power | 1 | W | No |
| 0x010B | Charge status | 1 | - | No (0=off,1=quick,2=CV,4=float,6=Li activate) |
| 0x010C-0x010D | Fault/alarm bits | 1 | - | 32-bit bitmap |
| 0x010E | Total charge power (PV+grid) | 1 | W | No |
| 0x010F | PV2 voltage | 0.1 | V | No |
| 0x0110 | PV2 current | 0.1 | A | No |
| 0x0111 | PV2 power | 1 | W | No |

## P02: Inverter Data (0x0200-0x0237) — Read-only

| Address | Name | Scale | Unit | Signed |
|---------|------|-------|------|--------|
| 0x0200-0x0203 | Current fault bits | 1 | - | 64-bit bitmap |
| 0x0204-0x0207 | Fault codes (4 slots) | 1 | - | 0=none |
| 0x020C-0x020E | Current time | 1 | - | Packed YMD/HMS |
| 0x0210 | Machine state | 1 | - | 0-10 (see below) |
| 0x0212 | Bus voltage | 0.1 | V | No |
| 0x0213 | Grid voltage L1 | 0.1 | V | No |
| 0x0214 | Grid current L1 | 0.1 | A | No |
| 0x0215 | Grid frequency | 0.01 | Hz | No |
| 0x0216 | Inverter voltage L1 | 0.1 | V | No |
| 0x0217 | Inverter current L1 | 0.1 | A | No |
| 0x0218 | Inverter frequency | 0.01 | Hz | No |
| 0x0219 | Load current L1 | 0.1 | A | No |
| 0x021A | Load power factor | 0.01 | - | Yes |
| 0x021B | Load active power L1 | 1 | W | No |
| 0x021C | Load apparent power L1 | 1 | VA | No |
| 0x021E | Mains charge current | 0.1 | A | No |
| 0x021F | Load ratio L1 | 1 | % | No |
| 0x0220 | Heatsink A temp (DC-DC) | 0.1 | C | Yes |
| 0x0221 | Heatsink B temp (DC-AC) | 0.1 | C | Yes |
| 0x0222 | Heatsink C temp (xfmr) | 0.1 | C | Yes |
| 0x0223 | Heatsink D / ambient | 0.1 | C | Yes |
| 0x0224 | PV charge current (batt side) | 0.1 | A | No |
| 0x022A | Grid voltage L2 | 0.1 | V | No | 3-phase only |
| 0x022B | Grid current L2 | 0.1 | A | No | 3-phase only |
| 0x022C | Inverter voltage L2 | 0.1 | V | No | 3-phase only |
| 0x022D | Inverter current L2 | 0.1 | A | No | 3-phase only |
| 0x022E | Load current L2 | 0.1 | A | No | 3-phase only |
| 0x022F | Load power L2 | 1 | W | No | 3-phase only |
| 0x0230 | Load apparent power L2 | 1 | VA | No | 3-phase only |
| 0x0231 | Load ratio L2 | 1 | % | No | 3-phase only |
| 0x0232 | Grid voltage L3 | 0.1 | V | No | 3-phase only |
| 0x0233 | Grid current L3 | 0.1 | A | No | 3-phase only |
| 0x0234 | Inverter voltage L3 | 0.1 | V | No | 3-phase only |
| 0x0235 | Inverter current L3 | 0.1 | A | No | 3-phase only |
| 0x0236 | Load current L3 | 0.1 | A | No | 3-phase only |
| 0x0237 | Load power L3 | 1 | W | No | 3-phase only |
| 0x0238 | Load apparent power L3 | 1 | VA | No | 3-phase only |
| 0x0239 | Load ratio L3 | 1 | % | No | 3-phase only |

### Machine States

0=delay, 1=wait, 2=init, 3=soft start, 4=mains, 5=inverter, 6=inv-to-mains, 7=mains-to-inv, 8=battery activate, 9=shutdown, 10=fault

## P03: Device Control (0xDF00-0xDF0D) — Write-only

| Address | Name | Value |
|---------|------|-------|
| 0xDF00 | Power ON/OFF | 0=off, 1=on |
| 0xDF01 | Reset | 1=reset |
| 0xDF02 | Restore defaults | 0xAA=restore |
| 0xDF03 | Clear current alarm | 1=clear |
| 0xDF04 | Clear statistics | 1=clear |
| 0xDF05 | Clear history | 1=clear |
| 0xDF08 | Sleep/wake | 0x5A5A=sleep, 0xA5A5=run |
| 0xDF0C | Generator switch | 1=switch |
| 0xDF0D | Immediate equalizing charge | 0=disable, 1=enable |

## P05: Battery Settings (0xE000-0xE04D) — Read/Write

Note: Voltage settings marked 12V-base are stored as 12V-equivalent values. Multiply by (systemVoltage/12) for actual voltage.

### Core Battery Parameters (0xE001-0xE025)

| Address | Name | Scale | Unit | 12V-base | Notes |
|---------|------|-------|------|----------|-------|
| 0xE001 | PV charge current limit | 0.1 | A | No | |
| 0xE002 | Nominal battery capacity | 1 | AH | No | |
| 0xE003 | System voltage | 1 | V | No | 12/24/36/48/0xFF=auto |
| 0xE004 | Battery type | 1 | - | No | 0=User,1=SLD,2=FLD,3=GEL,4=LFP,5=NCA,6=LFP(BMS) |
| 0xE005 | Over voltage (fast protect) | 0.1 | V | Yes | |
| 0xE006 | Limited charge voltage | 0.1 | V | Yes | |
| 0xE007 | Equalizing charge voltage | 0.1 | V | Yes | |
| 0xE008 | Boost/overcharge voltage | 0.1 | V | Yes | |
| 0xE009 | Float/overcharge-return voltage | 0.1 | V | Yes | |
| 0xE00A | Boost charge return voltage | 0.1 | V | Yes | |
| 0xE00B | Over discharge return voltage | 0.1 | V | Yes | |
| 0xE00C | Under-voltage warning | 0.1 | V | Yes | Alarm, no load cutoff |
| 0xE00D | Over discharge voltage | 0.1 | V | Yes | Load cutoff |
| 0xE00E | Limited discharge voltage | 0.1 | V | Yes | Immediate cutoff |
| 0xE00F | Cutoff SOC (charge/discharge) | 1 | % | No | Packed: hi=charge, lo=discharge |
| 0xE010 | Over discharge delay | 1 | s | No | 0-120s |
| 0xE011 | Equalizing charge time | 1 | min | No | 0-900min |
| 0xE012 | Boost charge time | 1 | min | No | 10-900min |
| 0xE013 | Equalizing charge interval | 1 | day | No | 0-255 days |
| 0xE014 | Temp compensation coefficient | 1 | mV/C/2V | No | May be invalid on some FW |
| 0xE015 | Charge max temp | 1 | C | No | Signed, may be invalid |
| 0xE016 | Charge min temp | 1 | C | No | Signed, may be invalid |
| 0xE017 | Discharge max temp | 1 | C | No | Signed, may be invalid |
| 0xE018 | Discharge min temp | 1 | C | No | Signed, may be invalid |
| 0xE019 | Battery heater start temp | 1 | C | No | Signed, may be invalid |
| 0xE01A | Battery heater stop temp | 1 | C | No | Signed, may be invalid |
| 0xE01B | Mains switching voltage | 0.1 | V | Yes | Switch load to AC below this |
| 0xE01C | Stop charge current (Li) | 0.1 | A | No | |
| 0xE01D | Stop charge SOC | 1 | % | No | Stop charging when SOC >= value |
| 0xE01E | Low SOC alarm | 1 | % | No | SOC alarm threshold |
| 0xE01F | SOC switch to mains | 1 | % | No | SBU: switch to mains when SOC <= value |
| 0xE020 | SOC switch to battery | 1 | % | No | SBU: switch to inverter when SOC >= value |
| 0xE022 | Inverter switching voltage | 0.1 | V | Yes | Switch back to inverter above this |
| 0xE023 | Equalizing charge timeout | 1 | min | No | 5-900min |
| 0xE024 | Li activation current | 0.1 | A | No | 0-20A |
| 0xE025 | BMS charge LC mode | 1 | - | No | 0/1/2 |

### Timed Charge/Discharge (0xE026-0xE033)

Time encoding: high byte = hours (0-23), low byte = minutes (0-59).

| Address | Name | Notes |
|---------|------|-------|
| 0xE026 | Charge start time 1 | Packed HH:MM |
| 0xE027 | Charge end time 1 | |
| 0xE028 | Charge start time 2 | |
| 0xE029 | Charge end time 2 | |
| 0xE02A | Charge start time 3 | |
| 0xE02B | Charge end time 3 | |
| 0xE02C | Timed charge enable | 0=disabled, 1=enabled |
| 0xE02D | Discharge start time 1 | Packed HH:MM |
| 0xE02E | Discharge end time 1 | |
| 0xE02F | Discharge start time 2 | |
| 0xE030 | Discharge end time 2 | |
| 0xE031 | Discharge start time 3 | |
| 0xE032 | Discharge end time 3 | |
| 0xE033 | Timed discharge enable | 0=disabled, 1=enabled |

### Per-Section SOC/Voltage/Power Cutoffs (0xE03B-0xE04D)

Added incrementally: SOC cutoffs in V1.94, voltage cutoffs + discharge power in V1.95, charge power + source in V1.96.

On protocol V1.07, addresses 0xE034-0xE03B are in reserved address space (read returns 0 but not functional). Addresses 0xE03C+ return "illegal address". Confirmed by probing individual registers on ASP48100U200-H with protocol V1.07 / SW V8.18.

| Address | Name | Scale | Unit |
|---------|------|-------|------|
| 0xE03B | Charge 1 stop SOC | 1 | % |
| 0xE03C | Charge 2 stop SOC | 1 | % |
| 0xE03D | Charge 3 stop SOC | 1 | % |
| 0xE03E | Discharge 1 stop SOC | 1 | % |
| 0xE03F | Discharge 2 stop SOC | 1 | % |
| 0xE040 | Discharge 3 stop SOC | 1 | % |
| 0xE041-0xE043 | Charge 1/2/3 stop voltage | 0.1 | V |
| 0xE044-0xE046 | Discharge 1/2/3 stop voltage | 0.1 | V |
| 0xE047-0xE049 | Discharge 1/2/3 max power | 10 | W |
| 0xE04A-0xE04C | Charge 1/2/3 max power | 10 | W |

### Other Extended Settings

| Address | Name | Notes |
|---------|------|-------|
| 0xE037 | PV grid-connected enable | 0=off-grid, 1=grid-connected |
| 0xE038 | GFCI enable | 0=disabled, 1=enabled |
| 0xE039 | PV power priority | 0=charging first, 1=load first |

## P07: Inverter Settings (0xE200-0xE221) — Read/Write

| Address | Name | Scale | Unit | Notes |
|---------|------|-------|------|-------|
| 0xE200 | RS485 address | 1 | - | 1-254 |
| 0xE201 | Parallel mode | 1 | - | 0=single,1-7=parallel modes |
| 0xE204 | Output priority | 1 | - | 0=SOL, 1=UTI, 2=SBU |
| 0xE205 | Mains charge current limit | 0.1 | A | 0-200A |
| 0xE206 | Equalizing charge enable | 1 | - | 0/1 |
| 0xE207 | N-G function enable | 1 | - | N-PE ground cable short enable |
| 0xE208 | Output voltage | 0.1 | V | 100-264V |
| 0xE209 | Output frequency | 0.01 | Hz | 45-65Hz |
| 0xE20A | Max charge current | 0.1 | A | 0-200A |
| 0xE20B | AC input range | 1 | - | 0=wide(APL), 1=narrow(UPS) |
| 0xE20C | Power saving mode | 1 | - | 0=disabled, 1=enabled |
| 0xE20D | Overload auto restart | 1 | - | 0/1 |
| 0xE20E | Over temp auto restart | 1 | - | 0/1 |
| 0xE20F | Charger priority | 1 | - | 0=CSO, 1=CUB, 2=SNU, 3=OSO |
| 0xE210 | Alarm enable | 1 | - | 0/1 |
| 0xE211 | Alarm on input loss | 1 | - | 0/1 |
| 0xE212 | Overload bypass enable | 1 | - | 0/1 |
| 0xE213 | Record fault enable | 1 | - | 0/1 |
| 0xE214 | BMS error stop enable | 1 | - | 0/1 |
| 0xE215 | BMS communication enable | 1 | - | 0=off, 1=485-BMS, 2=CAN-BMS |
| 0xE21B | BMS protocol | 1 | - | 0-30 |

## P08: Statistics (0xF000-0xF04B) — Read-only

| Address | Name | Scale | Unit |
|---------|------|-------|------|
| 0xF000-0xF006 | Last 7 days PV generation | 1 | AH |
| 0xF007-0xF00D | Last 7 days battery charge | 1 | AH |
| 0xF00E-0xF014 | Last 7 days battery discharge | 1 | AH |
| 0xF015-0xF01B | Last 7 days mains charge | 1 | AH |
| 0xF01C-0xF022 | Last 7 days load consumption | 0.1 | kWh |
| 0xF023-0xF029 | Last 7 days load from mains | 0.1 | kWh |
| 0xF02D | Battery charge today | 1 | AH |
| 0xF02E | Battery discharge today | 1 | AH |
| 0xF02F | PV generation today | 0.1 | kWh |
| 0xF030 | Load consumption today | 0.1 | kWh |
| 0xF031 | Total running days | 1 | days |
| 0xF032 | Total overdischarge count | 1 | - |
| 0xF033 | Total full charge count | 1 | - |
| 0xF034-0xF035 | Accumulated battery charge | 1 | AH (32-bit) |
| 0xF036-0xF037 | Accumulated battery discharge | 1 | AH (32-bit) |
| 0xF038-0xF039 | Accumulated PV generation | 0.1 | kWh (32-bit) |
| 0xF03A-0xF03B | Accumulated load consumption | 0.1 | kWh (32-bit) |
| 0xF046-0xF047 | Accumulated mains charge | 0.1 | kWh (32-bit) |
| 0xF04A | Accumulated inverter hours | 1 | h |
| 0xF04B | Accumulated bypass hours | 1 | h |

## P09: Fault History (0xF800-0xF9FF)

- 16 fault records at 0xF800-0xF8F0 (16 regs each): fault code, timestamp, 12-reg data snapshot
- 16 status records at 0xF900-0xF9F0 (same structure)

## Undocumented Registers (Discovered by Probing)

All registers in this section were discovered by sequential register probing on an ASP48100U200-H (protocol V1.07, SW V8.18, installed Sep 2025). They are **not present in any known SRNE protocol document** (V1.7, V1.96, or V2.08). Values shown are from a single probe session on 2026-03-24. **Do not write to these until their function is confirmed.**

### Probing Methodology

Every register in the following ranges was read individually via MODBUS function 0x03. Registers were classified as:
- **Active** — returned a non-zero value
- **Mapped** — returned 0 (addressable but possibly unused)
- **Unmapped** — returned error code 0x02 (illegal address)

Ranges probed: 0x0000-0x004F, 0x0050-0x00FF, 0x0100-0x013F, 0x0200-0x02FF, 0xDF00-0xDF0F, 0xE000-0xE05F, 0xE100-0xE149, 0xE200-0xE22F, 0xE400-0xE431, 0xF000-0xF05F, 0xF800-0xF9FF.

Ranges confirmed completely empty: 0x0000-0x0009, 0x0050-0x00FF, 0x0120-0x013F, 0x0240-0x02FF, 0xE060-0xE0FF, 0xF060-0xF0FF. The 0xDF00-0xDF0D range (device control) is readable (all zeros) but documented as write-only. The 0xF900-0xF9FF range (status history) is fully addressable but all zeros.

### P02 Inverter Data — Unknown Registers

| Address | Observed | Notes |
|---------|----------|-------|
| 0x0228 | 2644 | ×0.1 = 264.4V. Consistent across reads. Could be total/combined output voltage for split-phase, or a peak measurement. |
| 0x0229 | 2645 | Tracks close to 0x0228. Possibly a paired measurement (L1+L2 sum?). |

### P05 Battery Settings — Unknown Registers

| Address | Observed | Notes |
|---------|----------|-------|
| 0xE021 | 1000 | Between SOC Switch To Battery (0xE020=60) and Inverter Switching Voltage (0xE022). Documented as "reserved" in V1.96. Value 1000 — could be a threshold in mV, W, or mA. |

### P07 Inverter Settings — Unknown Registers

| Address | Observed | Notes |
|---------|----------|-------|
| 0xE217 | 12 | Between DC Load Switch (0xE216) and Derate Power (0xE218). V1.7 CSV lists this address as "start discharge time" (possibly legacy mapping). |

### 0xE100-0xE149 — Hidden Settings Block (39 active, 35 mapped-zero)

Entirely undocumented register block. Not present in any known protocol version. Possibly ASP-specific advanced configuration, parallel/split-phase tuning, or grid protection settings mapped to a different address range than V2.08's 0xE400 block.

| Address | Observed | Interpretation Guess |
|---------|----------|---------------------|
| 0xE101 | 12 | Config value |
| 0xE102 | 6 | Config value |
| 0xE103 | -28 (signed) | Calibration offset? |
| 0xE104 | -7 (signed) | Calibration offset? |
| 0xE106 | 4 | Config value |
| 0xE107 | -8 (signed) | Calibration offset? |
| 0xE108 | 22 | Config value |
| 0xE109 | 53 | Config value |
| 0xE10B | 3 | Config value |
| 0xE10C | -1 (signed) / 65535 | Sentinel or disabled flag? |
| 0xE114 | 220 | Likely nominal voltage (220V) |
| 0xE115 | 220 | Likely nominal voltage (220V) — L2 nominal? |
| 0xE116 | 45 | Could be minimum frequency (45Hz) |
| 0xE118 | 100 | Percentage (100%) |
| 0xE11F | 500 | ×0.1 = 50.0Hz? Or 500W? |
| 0xE120 | 2000 | ×0.1 = 200.0? Or 2000W? |
| 0xE121 | 200 | ×0.1 = 20.0? |
| 0xE122 | 10 | Seconds/count? |
| 0xE123 | 3750 | ×0.1 = 375.0? Could be a voltage/power threshold |
| 0xE124 | 5 | Config value |
| 0xE125 | 180 | ×0.1 = 18.0? Or 180 seconds? |
| 0xE126 | 6 | Config value |
| 0xE127 | 170 | ×0.1 = 17.0? |
| 0xE128 | 6 | Config value |
| 0xE129 | 416 | ×0.1 = 41.6? |
| 0xE12A | 300 | ×0.1 = 30.0? |
| 0xE12B | 10 | Config value |
| 0xE12C | 1150 | ×0.1 = 115.0V — likely over-voltage protection level |
| 0xE12D | 120 | ×0.1 = 12.0? Or 120 seconds? |
| 0xE12E | 1250 | ×0.1 = 125.0V — likely higher over-voltage protection |
| 0xE12F | 60 | ×0.1 = 6.0? Or 60 seconds? |
| 0xE130 | 850 | ×0.1 = 85.0V — likely under-voltage protection level |
| 0xE131 | 870 | ×0.1 = 87.0V — likely under-voltage return level |
| 0xE134 | 5 | Config value |
| 0xE136 | 46 | ×0.01 = 0.46? Or could be 46Hz (under-frequency) |
| 0xE13A | 8 | Config value |
| 0xE145 | 500 | ×0.1 = 50.0? Power or frequency threshold |
| 0xE146 | 1000 | ×0.1 = 100.0? Or 1000W |
| 0xE149 | 33217 (0x81C1) | Bitmap / config word |

**Likely grid protection thresholds** — registers 0xE12C-0xE131 have values consistent with multi-tier voltage protection levels for a 120V split-phase system: 115.0V, 125.0V, 85.0V, 87.0V (×0.1 scale). These may correspond to the grid protection settings that V2.08 documents at 0xE400+ but mapped to a different address range on the ASP firmware.

### 0xE400-0xE431 — Grid-Connection Parameters (6 active, 43 mapped-zero)

Partially populated. Most registers are zero, consistent with the device operating in off-grid mode (0xE037=0). The few active values may be defaults or carry over from factory config.

| Address | Observed | Notes |
|---------|----------|-------|
| 0xE401 | 100 | V2.08 docs: grid-connected power percentage |
| 0xE425 | 1 | Unknown config |
| 0xE426 | 15 | Unknown config |
| 0xE42B | 1000 | V2.08 docs: possibly CT ratio (1000:1) or smart meter address |
| 0xE42C | 100 | Percentage? |
| 0xE42D | 60 | Seconds? |

### P08 Statistics — Unknown Registers

| Address | Observed | Notes |
|---------|----------|-------|
| 0xF02A | 6659 (0x1A03) | Packed date: year=26, month=03 → March 2026. Possibly stats reset/period date. |
| 0xF02B | 6144 (0x1800) | Packed: day=24, hour=00 → 24th, midnight. Paired with 0xF02A. |
| 0xF040-0xF042 | 6409/7181/6430 | Packed timestamps: 0x1909=25/Sep, 0x1C0D=28/13(?), 0x191E=25/30(?). Some fields don't parse cleanly as dates. Installation was Sep 2025, so 25/09 fits. |
| 0xF043-0xF045 | 6409/7181/6430 | Identical to 0xF040-0xF042 — mirror/duplicate. |
| 0xF048 | 9285 | Could be high word of 32-bit accumulator paired with 0xF046 (accumulated mains charge). |
| 0xF04C | 70 | Small value, unknown. |
| 0xF04D | 79 | Small value, unknown. |
| 0xF04E | 35 | Small value, unknown. |
| 0xF050-0xF051 | 30679/0 | Possibly 32-bit accumulator (low word only). |
| 0xF052-0xF053 | 22412/0 | Possibly 32-bit accumulator. |
| 0xF054-0xF055 | 24862/0 | Possibly 32-bit accumulator. |

### P09 Fault History (0xF800-0xF8FF) — Confirmed Active

16 fault records confirmed populated by probing. Each record is 16 registers (0xF800-0xF80F, 0xF810-0xF81F, ... 0xF8F0-0xF8FF).

Record structure (inferred from repeating patterns):
- **[+0]** Fault code (e.g., 34=0x22, 33=0x21, 5=0x05)
- **[+1..+3]** Timestamp — packed date/time. Values like 6409 (0x1909=25/Sep) confirm installation date of Sep 2025.
- **[+4..+8]** Mostly zero — possibly additional fault context
- **[+9]** System state snapshot — values like 530, 529, 528, 515 (×0.1 = 53.0V, 52.9V = battery voltage at time of fault)
- **[+10..+14]** Additional snapshot data — voltage pairs (e.g., 2610/2613 ×0.1 = 261.0V/261.3V, split-phase output)
- **[+15]** Frequency — consistently 5000 or 4999 (×0.01 = 50.00Hz / 49.99Hz)

Status history (0xF900-0xF9FF) is fully addressable but all zeros — no status records logged.
