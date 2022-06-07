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

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"

	assert "github.com/stretchr/testify/require"
)

func TestPrintCaptions(t *testing.T) {
	printCaptions(t, "sample.eia608", "expected.eia608")
}

func TestPrintRollupCaptions(t *testing.T) {
	// This sample also has top-aligned captions (but we ignore that part for now)
	printCaptions(t, "rollup_sample.eia608", "rollup_expected.eia608")
}

func TestControlCharacterCaptions(t *testing.T) {
	printCaptions(t, "control_check_sample.eia608", "control_check_expected.eia608")
}

func printCaptions(t *testing.T, infile, outfile string) {
	assert := assert.New(t)

	// load and read the sample file, 2 bytes at a time
	data, err := os.ReadFile("testdata/" + infile)
	assert.Nil(err)
	cc := []uint16{}
	for len(data) > 2 {
		payload := binary.BigEndian.Uint16(data[0:2])
		cc = append(cc, payload)
		data = data[2:]
	}

	s := []string{}
	eia608 := EIA608Frame{}
	for _, c := range cc {
		ok, err := eia608.Decode(c)
		assert.Nil(err)
		if ok {
			s = append(s, eia608.String())
		}
	}
	str := strings.Join(s, "\n")

	// use this if the expected output needs to be updated
	os.WriteFile(outfile, []byte(str), 0644)

	// compare against expected result
	expected, err := os.ReadFile("testdata/" + outfile)
	assert.Nil(err)
	assert.Equal(string(expected), str)
}

func Test608_StateSnapshot(t *testing.T) {
	assert := assert.New(t)

	// snapshot of a non-initialized frame
	eia608 := EIA608Frame{}
	state := eia608.StateSnapshot()
	assert.NotNil(state)
	assert.Equal(EIA608State{Mode: Mode608_Unknown}, *state)

	// write to back buffer. state should still pull from (empty) front
	eia608.active = &eia608.back
	eia608.writeChar(72)
	eia608.writeChar(69)
	eia608.writeChar(76)
	eia608.writeChar(76)
	eia608.writeChar(79)
	state = eia608.StateSnapshot()
	assert.NotNil(state)
	assert.Equal(EIA608State{Mode: Mode608_PopOn, Row: 15}, *state)

	// swap front and back buffers, check contents
	eia608.front, eia608.back = eia608.back, eia608.front
	eia608.active = &eia608.back
	state = eia608.StateSnapshot()
	assert.NotNil(state)
	assert.Equal(EIA608State{Mode: Mode608_PopOn, Row: 15, Content: "hello"}, *state)

	// force paint-on
	eia608.front.state.Rollup = 1
	state = eia608.StateSnapshot()
	assert.NotNil(state)
	assert.Equal(EIA608State{Mode: Mode608_PaintOn, Row: 15, Rollup: 1, Content: "hello"}, *state)
}
