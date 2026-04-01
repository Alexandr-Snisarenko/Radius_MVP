package dict

import (
	"encoding/binary"
	"fmt"

	"layeh.com/radius"
)

func parseVendorType(desc *AttrDescriptor, vsa []byte) (uint32, error) {
	typeSize := vendorTypeSize(desc)
	if len(vsa) < typeSize {
		return 0, fmt.Errorf("radiusdict: short VSA type field")
	}
	return readUintBE(vsa[:typeSize]), nil
}

func extractVendorValue(desc *AttrDescriptor, vsa []byte) (radius.Attribute, error) {
	typeSize := vendorTypeSize(desc)
	lenSize := vendorLenSize(desc)
	headerSize := typeSize + lenSize

	if len(vsa) < typeSize {
		return nil, fmt.Errorf("radiusdict: short VSA type field")
	}
	if len(vsa) < headerSize {
		return nil, fmt.Errorf("radiusdict: short VSA header")
	}

	if lenSize == 0 {
		value := make(radius.Attribute, len(vsa[typeSize:]))
		copy(value, vsa[typeSize:])
		return value, nil
	}

	totalLength := int(readUintBE(vsa[typeSize:headerSize]))
	if totalLength < headerSize {
		return nil, fmt.Errorf("radiusdict: invalid VSA length %d", totalLength)
	}
	if totalLength > len(vsa) {
		return nil, fmt.Errorf("radiusdict: VSA length %d exceeds payload %d", totalLength, len(vsa))
	}

	value := make(radius.Attribute, totalLength-headerSize)
	copy(value, vsa[headerSize:totalLength])
	return value, nil
}

func buildVendorAttribute(desc *AttrDescriptor, raw radius.Attribute) (radius.Attribute, error) {
	typeSize := vendorTypeSize(desc)
	lenSize := vendorLenSize(desc)
	headerSize := typeSize + lenSize
	totalLength := headerSize + len(raw)

	if lenSize > 0 && totalLength > maxUintForSize(lenSize) {
		return nil, fmt.Errorf("radiusdict: vendor payload too large for length field")
	}
	if totalLength > 249 {
		return nil, fmt.Errorf("radiusdict: vendor payload too large")
	}

	out := make(radius.Attribute, totalLength)
	writeUintBE(out[:typeSize], desc.Type)
	if lenSize > 0 {
		writeUintBE(out[typeSize:headerSize], uint32(totalLength))
	}
	copy(out[headerSize:], raw)
	return out, nil
}

func vendorTypeSize(desc *AttrDescriptor) int {
	if desc == nil || desc.VendorTypeSize <= 0 {
		return 1
	}
	return desc.VendorTypeSize
}

func vendorLenSize(desc *AttrDescriptor) int {
	if desc == nil || desc.VendorLenSize < 0 {
		return 1
	}
	if desc == nil || desc.VendorLenSize == 0 {
		return 0
	}
	return desc.VendorLenSize
}

func readUintBE(b []byte) uint32 {
	switch len(b) {
	case 1:
		return uint32(b[0])
	case 2:
		return uint32(binary.BigEndian.Uint16(b))
	case 4:
		return binary.BigEndian.Uint32(b)
	default:
		var v uint32
		for _, part := range b {
			v = (v << 8) | uint32(part)
		}
		return v
	}
}

func writeUintBE(dst []byte, value uint32) {
	switch len(dst) {
	case 1:
		dst[0] = byte(value)
	case 2:
		binary.BigEndian.PutUint16(dst, uint16(value))
	case 4:
		binary.BigEndian.PutUint32(dst, value)
	default:
		for i := len(dst) - 1; i >= 0; i-- {
			dst[i] = byte(value)
			value >>= 8
		}
	}
}

func maxUintForSize(size int) int {
	switch size {
	case 0:
		return 0
	case 1:
		return 0xff
	case 2:
		return 0xffff
	case 4:
		return int(^uint32(0))
	default:
		v := 1
		for i := 0; i < size; i++ {
			v *= 256
		}
		return v - 1
	}
}
