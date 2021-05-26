package caption

// 0, 0 is bottom left
const (
	Rows = 15
	Cols = 32
)

type frameBufferChar struct {
	underline bool
	style     byte
	char      rune
}

type frameBuffer struct {
	data [Rows * Cols]frameBufferChar
}

func (b *frameBuffer) clear() {
	b.data = [Rows * Cols]frameBufferChar{}
}

func (b *frameBuffer) getChar(r, c uint) *frameBufferChar {
	if r >= Rows || c >= Cols {
		return nil
	}
	return &b.data[r*Rows+c]
}

func (b *frameBuffer) carrageReturn(n uint) {
	// s := (Rows - n) * Cols
	// e := (Rows - 1) * Cols
	// d := (Rows - n - 1) * Cols
	// b.clear()
	// copy(b.data[d:], b.data[s:e])
}

func (b *frameBuffer) setChar(r, c uint, char frameBufferChar) bool {
	val := b.getChar(r, c)
	if val != nil && *val != char {
		*val = char
		return true
	}
	return false
}

func (b *frameBuffer) String() string {
	var s string
	// TODO add formating, new lines, spaces, etx
	for i, c := range b.data {
		if c.char == 0 {
			continue
		}
		if 0 == i%Cols && len(s) > 0 {
			s += "\n"
		}

		s += string(c.char)
	}
	return s
}

type Frame struct {
	timestamp float64

	// State
	// Does every channel have its own state? If so, move this to the frameBuffer struct
	underline bool
	style     byte
	rollup    uint
	row, col  uint
	ccData    uint16

	// TODO add CC1-4 buffers
	front  frameBuffer
	back   frameBuffer
	active *frameBuffer
}

var parityTable = func() [128]byte {
	var table [128]byte
	bx := func(b, x int) byte { return byte(b << x & 0x80) }
	for i := 0; i < len(table); i++ {
		table[i] = byte((i & 0x7F)) | (0x80 ^ bx(i, 1) ^ bx(i, 2) ^ bx(i, 3) ^ bx(i, 4) ^ bx(i, 5) ^ bx(i, 6) ^ bx(i, 7))
	}
	return table
}()

var charMap = func() []rune {
	return []rune{
		// Basic NA
		' ', '!', '"', '#', '$', '%', '&', '’', '(', ')', 'á', '+', ',', '-', '.', '/', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', ':', ';', '<', '=', '>', '?', '@',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', '[', 'é', ']', 'í', 'ó', 'ú',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z', 'ç', '÷', 'Ñ', 'ñ', '█',
		// Special NA
		'®', '°', '½', '¿', '™', '¢', '£', '♪', 'à', ' ', 'è', 'â', 'ê', 'î', 'ô', 'û',
		// Extended Spanish/Miscellaneous
		'Á', 'É', 'Ó', 'Ú', 'Ü', 'ü', '‘', '¡', '*', '\'', '—', '©', '℠', '•', '“', '”',
		// Extended French
		'À', 'Â', 'Ç', 'È', 'Ê', 'Ë', 'ë', 'Î', 'Ï', 'ï', 'Ô', 'Ù', 'ù', 'Û', '«', '»',
		// Portuguese
		'Ã', 'ã', 'Í', 'Ì', 'ì', 'Ò', 'ò', 'Õ', 'õ', '{', '}', '\\', '^', '_', '|', '~',
		// German/Danish
		'Ä', 'ä', 'Ö', 'ö', 'ß', '¥', '¤', '¦', 'Å', 'å', 'Ø', 'ø', '┌', '┐', '└', '┘',
	}
}()

var rowMap = func() []uint {
	// This is accually reverse order. This way we can put row 0 at the bottom.
	return []uint{4, Rows, 14, 13, 12, 11, 3, 2, 1, 0, 10, 9, 8, 7, 6, 5}
}()

func ParityByte(ccData byte) byte {
	return parityTable[0x7F&ccData]
}

func ParityWord(ccData uint16) uint16 {
	a, b := parityTable[0x7F&byte(ccData>>8)], parityTable[0x7F&byte(ccData>>0)]
	return uint16(a)<<8 | uint16(b)
}

const (
	eia608_control_resume_caption_loading     = 0x1420
	eia608_control_backspace                  = 0x1421
	eia608_control_alarm_off                  = 0x1422
	eia608_control_alarm_on                   = 0x1423
	eia608_control_delete_to_end_of_row       = 0x1424
	eia608_control_roll_up_2                  = 0x1425
	eia608_control_roll_up_3                  = 0x1426
	eia608_control_roll_up_4                  = 0x1427
	eia608_control_resume_direct_captioning   = 0x1429
	eia608_control_text_restart               = 0x142A
	eia608_control_text_resume_text_display   = 0x142B
	eia608_control_erase_display_memory       = 0x142C
	eia608_control_carriage_return            = 0x142D
	eia608_control_erase_non_displayed_memory = 0x142E
	eia608_control_end_of_caption             = 0x142F

	eia608_tab_offset_1 = 0x1721
	eia608_tab_offset_2 = 0x1722
	eia608_tab_offset_3 = 0x1723
)

func isControl(ccData uint16) bool { return 0x1420 == (0x7670&ccData) || 0x1720 == (0x7770&ccData) }
func (f *Frame) backspace() {
	if f.col > 0 {
		f.col--
	}
	f.active.setChar(f.row, f.col, frameBufferChar{})
}

func (f *Frame) parseControl(ccData uint16) error {
	var cmd, cc uint16
	if 0 == 0x0200&ccData {
		cc = (ccData&0x0800)>>10 | (ccData&0x0100)>>8
		cmd = 0x167F & ccData
	} else {
		cc = (ccData & 0x0800) >> 11
		cmd = 0x177F & ccData
	}
	cc = cc // TODO!

	switch cmd {
	// Switch to paint on
	case eia608_control_resume_direct_captioning:
		f.rollup = 0
		f.active = &f.front
		return nil //LIBCAPTION_OK;

	case eia608_control_erase_display_memory:
		f.front.clear()
		return nil //LIBCAPTION_READY;

		// ROLL-UP
	case eia608_control_roll_up_2:
		f.rollup = 2
		f.active = &f.front
		return nil //LIBCAPTION_OK

	case eia608_control_roll_up_3:
		f.rollup = 3
		f.active = &f.front
		return nil //LIBCAPTION_OK

	case eia608_control_roll_up_4:
		f.rollup = 4
		f.active = &f.front
		return nil //LIBCAPTION_OK

	case eia608_control_carriage_return:
		// TODO!
		f.col = 0
		return nil //LIBCAPTION_OK
	case eia608_control_backspace:
		f.backspace()
		return nil //LIBCAPTION_OK
	case eia608_control_delete_to_end_of_row:
		for i := f.col; i < Cols; i++ {
			f.active.setChar(f.row, i, frameBufferChar{})
		}
		return nil //LIBCAPTION_OK

	// POP ON
	case eia608_control_resume_caption_loading:
		f.rollup = 0
		f.active = &f.back
		return nil //LIBCAPTION_OK;

	case eia608_control_erase_non_displayed_memory:
		f.back.clear()
		return nil //LIBCAPTION_OK;

	case eia608_control_end_of_caption:
		f.front, f.back = f.back, f.front
		f.back.clear()
		f.col, f.row = 0, 0
		f.active = &f.back
		return nil //LIBCAPTION_READY

	// cursor positioning
	case eia608_tab_offset_1:
		f.col += 1
		return nil //LIBCAPTION_OK;
	case eia608_tab_offset_2:
		f.col += 2
		return nil //LIBCAPTION_OK;
	case eia608_tab_offset_3:
		f.col += 3
		return nil //LIBCAPTION_OK;

	// Unhandled
	default:
		// case eia608_control_alarm_off:
		// case eia608_control_alarm_on:
		// case eia608_control_text_restart:
		// case eia608_control_text_resume_text_display:
		return nil //LIBCAPTION_OK

	}
}

const (
	eia608_style_white   = 0
	eia608_style_green   = 1
	eia608_style_blue    = 2
	eia608_style_cyan    = 3
	eia608_style_red     = 4
	eia608_style_yellow  = 5
	eia608_style_magenta = 6
	eia608_style_italics = 7
)

func isPreamble(ccData uint16) bool { return 0x1040 == (0x7040 & ccData) }
func (f *Frame) parsePreamble(ccData uint16) error {
	f.row = rowMap[((0x0700&ccData)>>7)|((0x0020&ccData)>>5)]
	// cc := !!(0x0800 & ccData) // TODO handle channels!
	f.underline = 0x0001&ccData == 1

	f.col, f.style = 0, eia608_style_white
	if 0x0010&ccData == 0 {
		f.style = byte((0x000E & ccData) >> 1)
	} else {
		f.col = uint(4 * ((0x000E & ccData) >> 1))
	}
	return nil
}

func isMidRowChange(ccData uint16) bool { return 0x1120 == (0x7770 & ccData) }
func (f *Frame) parseMidRowChange(ccData uint16) error {
	// cc := !!(0x0800 & ccData); TODO!
	if 0x1120 == (0x7770 & ccData) {
		f.style = byte((0x000E & ccData) >> 1)
		f.underline = 0x0001&ccData == 1
	}
	return nil
}

// returns true if the buffer changed
func (f *Frame) writeChar(i uint16) bool {
	char := '�'
	if int(i) < len(charMap) {
		char = charMap[i]
	}
	r := f.active.setChar(f.row, f.col, frameBufferChar{
		char:      char,
		underline: f.underline,
		style:     f.style,
	})
	if f.col < Cols {
		f.col++
	}
	return r
}

func isBasicNA(ccData uint16) bool   { return 0x0000 != (0x6000 & ccData) }
func isSpecialNA(ccData uint16) bool { return 0x1130 == (0x7770 & ccData) }
func isWesternEu(ccData uint16) bool { return 0x1220 == (0x7660 & ccData) }
func (f *Frame) parseText(ccData uint16) error {
	// Handle Basic NA BEFORE we strip the channel bit
	if isBasicNA(ccData) {
		f.writeChar((ccData >> 8) - 0x20)
		ccData &= 0x00FF
		if 0x0020 <= ccData && 0x0080 > ccData {
			// we got first char, yes. But what about second char?
			f.writeChar(ccData - 0x20)
		}
		return nil
	}

	// Check then strip second channel toggle
	// ccToggle := ccData & 0x0800 // TODO CC1-4
	ccData = ccData & 0xF7FF
	if isSpecialNA(ccData) {
		// Special North American character
		f.writeChar(ccData - 0x1130 + 0x60)
		return nil
	}

	if 0x1220 <= ccData && 0x1240 > ccData {
		// Extended Western European character set, Spanish/Miscellaneous/French
		f.backspace()
		f.writeChar(ccData - 0x1220 + 0x70)
		return nil

	}

	if 0x1320 <= ccData && 0x1340 > ccData {
		// Extended Western European character set, Portuguese/German/Danish
		f.backspace()
		f.writeChar(ccData - 0x1320 + 0x90)
		return nil
	}

	return nil //
}

func (f *Frame) Decode(ccData uint16, timestamp float64) error {
	// parity error, just skip it
	if ParityWord(ccData) != ccData {
		return nil
	}

	ccData &= 0x7F7F // strip off parity bits
	if ccData == 0 {
		return nil // padding
	}

	// TODO
	// if (0 > frame->timestamp || frame->timestamp == timestamp || LIBCAPTION_READY == frame->status) {
	//     frame->timestamp = timestamp;
	//     frame->status = LIBCAPTION_OK;
	// }

	// skip duplicate controll commands.
	if (isSpecialNA(ccData) || isControl(ccData)) && ccData == f.ccData {
		return nil
	}

	f.ccData = ccData
	if isControl(ccData) {
		return f.parseControl(ccData)
	}
	if isPreamble(ccData) {
		return f.parsePreamble(ccData)
	}
	if isMidRowChange(ccData) {
		return f.parseMidRowChange(ccData)
	}
	if f.active == nil {
		// We joind an in progrees stream, We must wait for a controll charcter to tell us what mode we are in
		return nil
	}
	if isBasicNA(ccData) || isSpecialNA(ccData) || isWesternEu(ccData) {
		return f.parseText(ccData)
	}
	return nil
}
