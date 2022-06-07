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
Parser for CEA-708 caption frames. This is an envelope for 608 captions.

References: https://en.wikipedia.org/wiki/CEA-708
						 https://shop.cta.tech/products/digital-television-dtv-closed-captioning
*/

import (
	"encoding/binary"
	"errors"
)

type cea708Provider int

const (
	Provider_DirectTV cea708Provider = 47
	Provider_ATSC     cea708Provider = 49
)

type cea708_cc_type byte

const (
	ntsc_cc_field_1    cea708_cc_type = 0
	ntsc_cc_field_2    cea708_cc_type = 1
	dvtcc_packet_data  cea708_cc_type = 2
	dvtcc_packet_start cea708_cc_type = 3
)

type cea708_cc_data struct {
	marker_bits byte // 'one_bit' and 'reserved' in CEA-708
	cc_valid    bool
	cc_type     cea708_cc_type
	cc_data     uint16
}

type cea708_user_data struct {
	process_em_data_flag bool
	process_cc_data_flag bool
	additional_data_flag bool
	cc_count             byte
	em_data              byte
	cc_data              []cea708_cc_data
}

type cea708 struct {
	provider            cea708Provider
	user_identifier     uint32
	user_data_type_code int

	// fields specific to H.264 SEI type 4 - user data registered by ITU-T T.35
	itu_t_t35_country_code                int
	itu_t_t35_country_code_extension_byte byte
}

// CEA708ToCCData takes a H.264 SEI payload of "Registered User Data ITU-T T.35"
// and returns a list of 608 bytes that have passed validity checking
func CEA708ToCCData(data []byte) ([]uint16, error) {
	user_data, err := parseCEA708(data)
	if err != nil {
		return nil, err
	}

	// Sometimes providers insert weird crud, so filter out the invalid ones
	// TODO prob need to fix for multi-language
	return printableCCData(user_data), nil
}

func isPrintable(cd *cea708_cc_data) bool {
	return cd.cc_valid && cd.cc_type == ntsc_cc_field_1
}

func printableCCData(ud *cea708_user_data) []uint16 {
	d := []uint16{}
	for _, cd := range ud.cc_data {
		if !isPrintable(&cd) {
			continue
		}
		d = append(d, cd.cc_data)
	}
	return d
}

func parseCEA708UserData(data []byte) (*cea708_user_data, error) {

	if len(data) <= 2 {
		return nil, errors.New("insufficient cea708 user data")
	}

	ud := cea708_user_data{}
	ud.process_em_data_flag = data[0]&0x80 == 0x80
	ud.process_cc_data_flag = data[0]&0x40 == 0x40
	ud.additional_data_flag = data[0]&0x20 == 0x20
	ud.cc_count = data[0] & 0x1F
	ud.em_data = data[1]
	ud.cc_data = make([]cea708_cc_data, 0, 32)

	for i := 2; i+2 < len(data) && byte(len(ud.cc_data)) < ud.cc_count; i += 3 {
		d := data[i : i+3]
		cc_data := cea708_cc_data{
			marker_bits: d[0] >> 3,
			cc_valid:    d[0]&0x40 == 0x40,
			cc_type:     cea708_cc_type(d[0] & 0x3),
			cc_data:     binary.BigEndian.Uint16(d[1:3]),
		}
		ud.cc_data = append(ud.cc_data, cc_data)
	}

	// some checking
	if len(ud.cc_data) != int(ud.cc_count) {
		return nil, errors.New("mismatched cc count")
	}

	return &ud, nil
}

// Parses a CEA-708 packet.
func parseCEA708(data []byte) (*cea708_user_data, error) {
	sz := len(data)
	if sz < 4 {
		return nil, errors.New("insuffucient data to detect payload size")
	}
	i := 0
	c := cea708{}

	c.itu_t_t35_country_code = int(data[i])
	i += 1
	if c.itu_t_t35_country_code == 0xFF {
		c.itu_t_t35_country_code_extension_byte = data[i]
		i += 1
	}
	c.provider = cea708Provider(binary.BigEndian.Uint16(data[i : i+2]))
	i += 2
	if c.provider == Provider_ATSC {
		if sz-i < 4 {
			return nil, errors.New("insufficient data to read user identifier")
		}
		c.user_identifier = uint32(binary.BigEndian.Uint32(data[i : i+4]))
		i += 4
	}
	if 0 == c.provider && 0 == c.itu_t_t35_country_code {
		// where country and provider are zero
		// only seems to come up in onCaptionInfo
		// h264 spec seems to describe this
		i += 1
	}
	if c.provider == Provider_ATSC || c.provider == Provider_DirectTV { // ATSC or DirecTV
		if sz-i <= 1 {
			return nil, errors.New("insufficient data to read provider type code")
		}
		c.user_data_type_code = int(data[i])
		i += 1
	}

	if 3 == c.user_data_type_code && sz-i >= 2 {
		// parse user data type structure
		return parseCEA708UserData(data[i:])
	}

	return nil, errors.New("trailing user data")
}
