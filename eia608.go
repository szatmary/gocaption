package captions

/**********************************************************************************************/
/* The MIT License                                                                            */
/*                                                                                            */
/* Copyright 2016-2017 Twitch Interactive, Inc. or its affiliates. All Rights Reserved.       */
/* golang Port Copyright (c) 2022 Mux (mux.com)                                                      */
/*                                                                                            */
/* Permission is hereby granted, free of charge, to any person obtaining a copy               */
/* of this software and associated documentation files (the "Software"), to deal              */
/* in the Software without restriction, including without limitation the rights               */
/* to use, copy, modify, merge, publish, distribute, sublicense, and/or sell                  */
/* copies of the Software, and to permit persons to whom the Software is                      */
/* furnished to do so, subject to the following conditions:                                   */
/*                                                                                            */
/* The above copyright notice and this permission notice shall be included in                 */
/* all copies or substantial portions of the Software.                                        */
/*                                                                                            */
/* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR                 */
/* IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,                   */
/* FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE                */
/* AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER                     */
/* LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,              */
/* OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN                  */
/* THE SOFTWARE.                                                                              */
/**********************************************************************************************/


/*
Parser for EIA / CEA-608 captions.

References: https://shop.cta.tech/products/line-21-data-services
            https://www.govinfo.gov/content/pkg/CFR-2007-title47-vol1/pdf/CFR-2007-title47-vol1-sec15-119.pdf
*/

import (
	"strings"
)

// 0, 0 is bottom left
const (
	Rows = 15
	Cols = 32
)

type Mode608 int

const (
	Mode608_Unknown Mode608 = iota
	Mode608_PopOn
	Mode608_PaintOn
)

// represents a snapshot of the current 608 state.
type EIA608State struct {
	Mode Mode608
	// Rollup can be used to determine the mode we're in.
	Rollup int
	// Configured row position (not current cursor position)
	Row int
	// Configured column position (not current cursor position)
	Col int
	// The captions themselves.
	Content string
}

// EIA608Frame is an opaque type holding information about 608 frames.
type EIA608Frame struct {
	// State
	// Does every channel have its own state? If so, move this to the frameBuffer struct
	underline bool
	style     byte
	row, col  uint
	ccData    uint16

	// TODO add CC1-4 buffers
	front  frameBuffer
	back   frameBuffer
	active *frameBuffer
}

// Decode a single, 2-byte 608 packet. This accumulates data into a frame.
// If the frame is ready for display, returns true. Otherwise, false or error.
func (f *EIA608Frame) Decode(ccData uint16) (bool, error) {
	// parity error, just skip it
	if parityWord(ccData) != ccData {
		return false, nil
	}

	ccData &= 0x7F7F // strip off parity bits
	if ccData == 0 {
		return false, nil // padding
	}

	// skip duplicate control commands.
	if (isSpecialNA(ccData) || isControl(ccData)) && ccData == f.ccData {
		return false, nil
	}

	f.ccData = ccData
	if isControl(ccData) {
		return f.parseControl(ccData), nil
	}
	if f.active == nil {
		// We joined an in-progress stream, We must wait for a control character to tell us what mode we are in
		return false, nil
	}

	if isPreamble(ccData) {
		return false, f.parsePreamble(ccData)
	}
	if isMidRowChange(ccData) {
		return false, f.parseMidRowChange(ccData)
	}
	if isBasicNA(ccData) || isSpecialNA(ccData) || isWesternEu(ccData) {
		if err := f.parseText(ccData); err != nil {
			return false, err
		}
		return f.active.state.Rollup > 0, nil
	}
	return false, nil // TODO error here?
}

// String returns the front (display) buffer of the eia608 frame as a string
func (f *EIA608Frame) String() string {
	return f.front.String()
}

// Represents a snapshot of the 608 state for the front (display) buffer.
func (f *EIA608Frame) StateSnapshot() *EIA608State {
	// unknown mode if active has not yet been set
	if f.active == nil {
		return &EIA608State{
			Mode: Mode608_Unknown,
		}
	}
	frameBuffer := f.front
	mode := Mode608_PopOn
	if frameBuffer.state.Rollup > 0 {
		// for now. may need to add a non-paint on rollup mode later
		mode = Mode608_PaintOn
	}
	return &EIA608State{
		Mode:   mode,
		Rollup: frameBuffer.state.Rollup,
		// TODO rows might need to accommodate current cursor row (newlines)?
		//      rows are not currently used in practice but check if this changes
		Row:     Rows - frameBuffer.state.Row,
		Col:     frameBuffer.state.Col,
		Content: frameBuffer.String(),
	}
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

func parityByte(ccData byte) byte {
	return parityTable[0x7F&ccData]
}

func parityWord(ccData uint16) uint16 {
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

func (f *EIA608Frame) backspace() {
	if f.col > 0 {
		f.col--
	}
	f.active.setChar(f.row, f.col, frameBufferChar{})
}

func (f *EIA608Frame) parseControl(ccData uint16) bool {
	var cmd uint16
	if 0 == 0x0200&ccData {
		//cc = (ccData&0x0800)>>10 | (ccData&0x0100)>>8 // TODO cc channel
		cmd = 0x167F & ccData
	} else {
		//cc = (ccData & 0x0800) >> 11 // TODO cc channel
		cmd = 0x177F & ccData
	}

	switch cmd {
	// Switch to paint on
	case eia608_control_resume_direct_captioning:
		f.active = &f.front
		f.active.state.Rollup = 1
		return false //LIBCAPTION_OK;

	case eia608_control_erase_display_memory:
		f.front.clear()
		return true //LIBCAPTION_READY;

		// ROLL-UP
	case eia608_control_roll_up_2:
		f.active = &f.front
		f.active.state.Rollup = 2
		return false //LIBCAPTION_OK

	case eia608_control_roll_up_3:
		f.active = &f.front
		f.active.state.Rollup = 3
		return false //LIBCAPTION_OK

	case eia608_control_roll_up_4:
		f.active = &f.front
		f.active.state.Rollup = 4
		return false //LIBCAPTION_OK

	case eia608_control_carriage_return:
		if f.active == nil {
			return false
		}
		f.col = 0
		f.active.carriageReturn(f.row)
		f.active.state.Col = 0
		return false //LIBCAPTION_OK
	case eia608_control_backspace:
		if f.active == nil {
			return false
		}
		f.backspace()
		return false //LIBCAPTION_OK
	case eia608_control_delete_to_end_of_row:
		if f.active == nil {
			return false
		}
		for i := f.col; i < Cols; i++ {
			f.active.setChar(f.row, i, frameBufferChar{})
		}
		return false //LIBCAPTION_OK

	// POP ON
	case eia608_control_resume_caption_loading:
		f.active = &f.back
		f.active.state.Rollup = 0
		return false //LIBCAPTION_OK;

	case eia608_control_erase_non_displayed_memory:
		f.back.clear()
		return false //LIBCAPTION_OK;

	case eia608_control_end_of_caption:
		f.front, f.back = f.back, f.front
		f.back.clearState()
		// TODO hoist cursors (f.col, f.row) into the state struct
		f.col, f.row = 0, 0
		f.active = &f.back
		return true //LIBCAPTION_READY

	// cursor positioning
	case eia608_tab_offset_1:
		if f.active == nil {
			return false
		}
		// TODO ideally f.col (current cursor position) would be within state itself
		f.col += 1
		f.active.state.Col += 1
		return false //LIBCAPTION_OK;
	case eia608_tab_offset_2:
		if f.active == nil {
			return false
		}
		// TODO ideally f.col (current cursor position) would be within state itself
		f.col += 2
		f.active.state.Col += 2
		return false //LIBCAPTION_OK;
	case eia608_tab_offset_3:
		if f.active == nil {
			return false
		}
		// TODO ideally f.col (current cursor position) would be within state itself
		f.col += 3
		f.active.state.Col += 3
		return false //LIBCAPTION_OK;

	// Unhandled
	default:
		// case eia608_control_alarm_off:
		// case eia608_control_alarm_on:
		// case eia608_control_text_restart:
		// case eia608_control_text_resume_text_display:
		return false //LIBCAPTION_OK

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
func (f *EIA608Frame) parsePreamble(ccData uint16) error {
	f.row = rowMap[((0x0700&ccData)>>7)|((0x0020&ccData)>>5)]
	// cc := !!(0x0800 & ccData) // TODO handle channels!
	f.underline = 0x0001&ccData == 1

	f.col, f.style = 0, eia608_style_white
	if 0x0010&ccData == 0 {
		f.style = byte((0x000E & ccData) >> 1)
	} else {
		f.col = uint(4 * ((0x000E & ccData) >> 1))
	}
	f.active.state.Row = int(f.row)
	f.active.state.Col = int(f.col)
	return nil
}

func isMidRowChange(ccData uint16) bool { return 0x1120 == (0x7770 & ccData) }
func (f *EIA608Frame) parseMidRowChange(ccData uint16) error {
	// cc := !!(0x0800 & ccData); TODO cc channel
	if 0x1120 == (0x7770 & ccData) {
		f.style = byte((0x000E & ccData) >> 1)
		f.underline = 0x0001&ccData == 1
	}
	return nil
}

// returns true if the buffer changed
func (f *EIA608Frame) writeChar(i uint16) bool {
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

func (f *EIA608Frame) parseText(ccData uint16) error {
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

type frameBufferChar struct {
	underline bool
	style     byte
	char      rune
}

type frameBufferRow [Cols]frameBufferChar

type frameBuffer struct {
	state EIA608State
	data  [Rows]frameBufferRow
}

func (b *frameBuffer) clear() {
	b.data = [Rows]frameBufferRow{}
}

func (b *frameBuffer) clearState() {
	b.clear()
	b.state.Row = 0
	b.state.Col = 0
}

func (b *frameBuffer) getChar(r, c uint) *frameBufferChar {
	if r >= Rows || c >= Cols {
		return nil
	}
	return &b.data[r][c]
}

func (b *frameBuffer) carriageReturn(row uint) {
	rollups := uint(b.state.Rollup)
	if row+rollups >= Rows+1 || row+rollups <= 0 {
		return
	}
	n := rollups - 1
	for i := uint(0); i < n; i++ {
		idx := row + n - i
		b.data[idx] = b.data[idx-1]
	}
	b.data[row] = [Cols]frameBufferChar{} // clear last row
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
	var s []string
	// TODO add formating, new lines, spaces, etc
	for _, row := range b.data {
		var sb strings.Builder
		for _, c := range row {
			if c.char == 0 {
				continue
			}
			sb.WriteRune(c.char)
		}
		if sb.Len() > 0 {
			s = append(s, sb.String())
		}
	}

	// reverse the list since we have the bottom first
	last := len(s) - 1
	for i := 0; i < len(s)/2; i++ {
		s[i], s[last-i] = s[last-i], s[i]
	}

	return strings.Join(s, "\n")
}
