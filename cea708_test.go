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
	"fmt"
	"os"
	"strings"
	"testing"

	assert "github.com/stretchr/testify/require"
)

func print_bool(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func print_byte(b byte) string {
	return fmt.Sprintf("%d", b)
}

func print_uint16(v uint16) string {
	return fmt.Sprintf("%d", v)
}

func print_cc_data(cc_data cea708_cc_data) string {
	s := []string{}
	s = append(s, print_byte(cc_data.marker_bits))
	s = append(s, print_bool(cc_data.cc_valid))
	s = append(s, print_byte(byte(cc_data.cc_type)))
	s = append(s, print_uint16(cc_data.cc_data))
	return strings.Join(s, "|")
}

func print_user_data(user_data *cea708_user_data) string {
	s := []string{}
	s = append(s, print_bool(user_data.process_em_data_flag))
	s = append(s, print_bool(user_data.process_cc_data_flag))
	s = append(s, print_bool(user_data.additional_data_flag))
	s = append(s, print_byte(user_data.cc_count))
	s = append(s, print_byte(user_data.em_data))
	for _, cc := range user_data.cc_data {
		s = append(s, print_cc_data(cc))
	}
	return strings.Join(s, ",")
}

func Test_parseCEA708(t *testing.T) {
	assert := assert.New(t)
	data, err := os.ReadFile("testdata/sample.cea708")
	assert.Nil(err)
	// read the sample file. simple 4 byte size + data
	payloads := [][]byte{}
	for len(data) > 4 {
		sz := binary.BigEndian.Uint32(data[0:4])
		payloads = append(payloads, data[4:4+sz+1])
		data = data[4+sz:]
	}
	assert.Empty(data)
	// now check the payloads
	s := []string{"process_em_data_flag,process_cc_data_flag,additional_data_flag,cc_count,em_data,[]cc_data"}
	for _, p := range payloads {
		user_data, err := parseCEA708(p)
		assert.Nil(err)
		s = append(s, print_user_data(user_data))
	}
	str := strings.Join(s, "\n")

	// use this if the expected output needs to be updated
	err = os.WriteFile("expected.cea708", []byte(str), 0666)
	assert.Nil(err)

	// compare against expected result
	expected, err := os.ReadFile("testdata/expected.cea708")
	assert.Nil(err)
	assert.Equal(string(expected), str)
}
