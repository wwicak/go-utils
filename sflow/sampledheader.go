package sflow

import (
	"encoding/binary"
)

type SampledHeader struct {
	Protocol       uint32
	FrameLength    uint32
	PayloadRemoved uint32
	Header         []byte
}

func (sh *SampledHeader) Parse(data []byte) error {
	if len(data) < 16 {
		return ErrTooShort
	}
	sh.Protocol = binary.BigEndian.Uint32(data[0:4])
	sh.FrameLength = binary.BigEndian.Uint32(data[4:8])
	sh.PayloadRemoved = binary.BigEndian.Uint32(data[8:12])
	headerlength := binary.BigEndian.Uint32(data[12:16])
	if uint32(len(data[16:])) < headerlength {
		return ErrOutOfBounds
	}
	sh.Header = make([]byte, headerlength)
	copy(sh.Header, data[16:16+headerlength])
	return nil
}

func (*SampledHeader) FlowType() uint32 {
	return SampledHeaderType
}

func (sh *SampledHeader) SampledIPv4() *SampledIPV4 {
	var sampleIPv4 *SampledIPV4
	switch sh.Protocol {
	case 1:
		ethernetHeaderSize := 14
		header := sh.Header
		if len(header) < ethernetHeaderSize {
			return nil
		}
		switch binary.BigEndian.Uint16(header[12:14]) {
		case 0x8100:
			if len(header) < 18 {
				return nil
			}
			ethernetHeaderSize += 4
		case 0x88a8:
			if len(header) < 22 {
				return nil
			}
			ethernetHeaderSize += 8
		}
		if len(header) < ethernetHeaderSize {
			return nil
		}
		sampleIPv4 = &SampledIPV4{}
		if err := sampleIPv4.ParseFromIPHeader(header[ethernetHeaderSize:]); err != nil {
			return nil
		}
	case 11:
		sampleIPv4 = &SampledIPV4{}
		if err := sampleIPv4.ParseFromIPHeader(sh.Header); err != nil {
			return nil
		}
	}

	return sampleIPv4
}
