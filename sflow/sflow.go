package sflow

import (
	"encoding/binary"
	"errors"
)

const (
	FlowSampleType uint32 = iota + 1
	CounterSamplesType
	FlowSampleExpandedType
	CountersSampleExpandedType
)

const (
	SampledUnknownType uint32 = iota
	SampledHeaderType
	SampledEthernetType
	SampledIPV4Type
	SampledIPV6Type
)

const (
	ExtendedSwitchType uint32 = iota + 1001
	ExtendedRouterType
	ExtendedGatewayType
	ExtendedUserType
	ExtendedURLType
	ExtendedMPLSType
	ExtendedNATType
	ExtendedMPLSTunnelType
	ExtendedMPLSVCType
	ExtendedMPLSFTNType
	ExtendedMPLSLDPFECType
	ExtendedVLANTunnelType
)

const (
	IfCountersType uint32 = iota + 1
	EthernetCountersType
	TokenringCountersType
	VGCountersType
	VlanCountersType
)

const (
	ProcessorType = 1001
)

type Sample interface {
	SampleType() uint32
	Parse([]byte) error
}

type Counter interface {
	CounterType() uint32
	Parse([]byte) error
}

type Flow interface {
	FlowType() uint32
	Parse([]byte) error
}

type Header struct {
	Version        uint32
	AddressType    uint32
	AgentAddress   [4]byte
	SubAgentID     uint32
	SequenceNumber uint32
	SysUptime      uint32
	NumSamples     uint32
}

func (h *Header) Parse(data []byte) ([]byte, error) {
	if len(data) < 28 {
		return nil, ErrTooShort
	}

	h.Version = binary.BigEndian.Uint32(data[0:4])
	h.AddressType = binary.BigEndian.Uint32(data[4:8])
	copy(h.AgentAddress[:], data[8:12])
	h.SubAgentID = binary.BigEndian.Uint32(data[12:16])
	h.SequenceNumber = binary.BigEndian.Uint32(data[16:20])
	h.SysUptime = binary.BigEndian.Uint32(data[20:24])
	h.NumSamples = binary.BigEndian.Uint32(data[24:28])
	return data[28:], nil
}

type DataFormat struct {
	Format uint32
	Length uint32
}

var (
	ErrTooShort    = errors.New("sflow: data is too short")
	ErrOutOfBounds = errors.New("sflow: out of bounds")
)

func parseBigEndianUint32(data []byte) (uint32, error) {
	if len(data) < 4 {
		return 0, ErrTooShort
	}

	return binary.BigEndian.Uint32(data[0:4]), nil
}

func (h *DataFormat) Parse(data []byte) ([]byte, error) {
	if len(data) < 8 {
		return nil, ErrTooShort
	}
	h.Format = binary.BigEndian.Uint32(data[0:4])
	h.Length = binary.BigEndian.Uint32(data[4:8])
	if uint32(len(data[8:])) < h.Length {
		return nil, ErrOutOfBounds
	}

	return data[8:], nil
}

type CountersSample struct {
	SequenceNumber uint32
	SourceId       uint32
	NumSamples     uint32
}

func (h *CountersSample) Parse(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, ErrTooShort
	}
	h.SequenceNumber = binary.BigEndian.Uint32(data[0:4])
	h.SourceId = binary.BigEndian.Uint32(data[4:8])
	h.NumSamples = binary.BigEndian.Uint32(data[8:12])
	return data[12:], nil
}

func (df *DataFormat) ParseFlow(data []byte) (Flow, []byte, error) {
	if uint32(len(data)) < df.Length {
		return nil, nil, ErrOutOfBounds
	}
	body := data[:df.Length]
	rest := data[df.Length:]

	var flow Flow
	switch df.Format {
	default:
		flow = &SampledUnknown{Type: df.Format}
	case SampledHeaderType:
		flow = &SampledHeader{}
	case SampledIPV4Type:
		flow = &SampledIPV4{}
	case SampledIPV6Type:
		flow = &SampledIPV6{}
	}

	if flow == nil {
		return nil, rest, nil
	}

	if err := flow.Parse(body); err != nil {
		return nil, nil, err
	}

	return flow, rest, nil
}

func (h *Header) ParseSamples(data []byte) ([]Sample, error) {
	dfs := []DataFormat{}
	samples := []Sample{}
	var sample Sample
	var err error
	for i := uint32(0); i < h.NumSamples; i++ {
		df := DataFormat{}
		data, err = df.Parse(data)
		if err != nil {
			return nil, err
		}
		dfs = append(dfs, df)
		sample, data, err = df.ParseSample(data)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}

	return samples, nil
}

func (df *DataFormat) ParseSample(data []byte) (Sample, []byte, error) {
	if uint32(len(data)) < df.Length {
		return nil, nil, ErrOutOfBounds
	}
	body := data[:df.Length]
	rest := data[df.Length:]

	var sample Sample
	switch df.Format {
	case CounterSamplesType:
		sample = &CounterSamples{}
	case FlowSampleType:
		sample = &FlowSample{}
	case FlowSampleExpandedType:
		sample = &FlowSampleExpanded{}
	}

	if sample == nil {
		return nil, rest, nil
	}

	if err := sample.Parse(body); err != nil {
		return nil, nil, err
	}

	return sample, rest, nil
}

func (df *DataFormat) ParseCounter(data []byte) (Counter, []byte, error) {
	if uint32(len(data)) < df.Length {
		return nil, nil, ErrOutOfBounds
	}
	body := data[:df.Length]
	rest := data[df.Length:]

	var counter Counter
	switch df.Format {
	default:
		counter = &CounterUnknown{Type: df.Format}
	case IfCountersType:
		counter = &IfCounter{}
	case EthernetCountersType:
		counter = &EthernetCounter{}
	case TokenringCountersType:
		counter = &TokenringCounters{}
	case VGCountersType:
		counter = &VGCounters{}
	case VlanCountersType:
		counter = &VlanCounters{}
	case ProcessorType:
		counter = &Processor{}
	}

	if counter == nil {
		return nil, rest, nil
	}

	if err := counter.Parse(body); err != nil {
		return nil, nil, err
	}

	return counter, rest, nil
}
